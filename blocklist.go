package main

import (
    "bufio"
    "encoding/json"
    "errors"
    "io"
    "net"
    "net/http"
    "os"
    "path/filepath"
    "regexp"
    "strings"
    "sync"
    "time"
    "log"
)

// BlocklistManager loads and manages blocklist files from a directory.
// Each file is a plain text file with one pattern per line. Patterns can
// include '*' wildcards (see docs). Lines starting with '#' and blank lines are ignored.
type BlocklistManager struct {
    dir      string
    mu       sync.RWMutex
    lists    map[string][]string       // raw patterns per list filename (no ext)
    compiled []*regexp.Regexp         // combined compiled regexps for fast checks
    // analytics
    statsMu       sync.RWMutex
    queries       int
    blockedQueries int
    domainHits    map[string]int // counts for blocked domains
    allHits       map[string]int // counts for all queried domains
    clientHits    map[string]int // counts per client IP
    // recent queries (simple append-only ring)
    recentMu      sync.Mutex
    recent        []QueryEntry
    recentCap     int
    // persistent logs file (JSON lines)
    logPath       string
    logMu         sync.Mutex
}

// QueryEntry is a single DNS query record stored for recent logs.
type QueryEntry struct {
    Time    time.Time `json:"time"`
    Domain  string    `json:"domain"`
    Client  string    `json:"client"`
    Blocked bool      `json:"blocked"`
}

// NewBlocklistManager ensures dir exists, loads all lists and compiles patterns.
func NewBlocklistManager(dir string) (*BlocklistManager, error) {
    if dir == "" {
        return nil, errors.New("empty directory")
    }
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return nil, err
    }
        bm := &BlocklistManager{
            dir: dir,
            lists: make(map[string][]string),
            domainHits: make(map[string]int),
            clientHits: make(map[string]int),
            allHits: make(map[string]int),
            recent: make([]QueryEntry, 0, 500),
            recentCap: 500,
        }
    if err := bm.LoadAll(); err != nil {
        return nil, err
    }
    // logs file inside the same directory
    bm.logPath = filepath.Join(dir, "logs.jsonl")
    return bm, nil
}

// LoadAll reads all .txt files from the directory and compiles patterns.
func (b *BlocklistManager) LoadAll() error {
    entries, err := os.ReadDir(b.dir)
    if err != nil {
        return err
    }

    lists := make(map[string][]string)
    for _, e := range entries {
        if e.IsDir() {
            continue
        }
        name := e.Name()
        if !strings.HasSuffix(strings.ToLower(name), ".txt") {
            continue
        }
        path := filepath.Join(b.dir, name)
        f, err := os.Open(path)
        if err != nil {
            continue
        }
        patterns, _ := readLines(f)
        _ = f.Close()
        base := strings.TrimSuffix(name, filepath.Ext(name))
        lists[base] = patterns
    }

    // compile into regexps
    compiled := make([]*regexp.Regexp, 0)
    for _, pats := range lists {
        for _, p := range pats {
            if p = strings.TrimSpace(p); p == "" {
                continue
            }
            re, err := patternToRegexp(p)
            if err == nil && re != nil {
                compiled = append(compiled, re)
            }
        }
    }

    b.mu.Lock()
    defer b.mu.Unlock()
    b.lists = lists
    b.compiled = compiled
    return nil
}

// IsBlocked returns true if the domain matches any compiled pattern.
// domain should be a host like "tracker.example.com" (trailing dot is tolerated).
func (b *BlocklistManager) IsBlocked(domain string) bool {
    d := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
    b.mu.RLock()
    defer b.mu.RUnlock()
    for _, re := range b.compiled {
        if re.MatchString(d) {
            return true
        }
    }
    return false
}

// AddFileToList downloads the URL (raw text) and appends unique entries into the named list.
// If createIfMissing is true it creates a new list file.
func (b *BlocklistManager) AddFileToList(listName, url string, createIfMissing bool) (int, error) {
    if listName == "" || url == "" {
        return 0, errors.New("missing list name or url")
    }

    client := &http.Client{Timeout: 15 * time.Second}
    resp, err := client.Get(url)
    if err != nil {
        log.Printf("AddFileToList: failed to GET %s: %v", url, err)
        return 0, err
    }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return 0, errors.New("failed to fetch file: " + resp.Status)
    }

    newLines, _ := readLines(resp.Body)
    // filter and normalize lines
    set := make(map[string]struct{})

    path := filepath.Join(b.dir, listName+".txt")
    // read existing
    if f, err := os.Open(path); err == nil {
        old, _ := readLines(f)
        _ = f.Close()
        for _, l := range old {
            s := normalizePattern(l)
            if s != "" {
                set[s] = struct{}{}
            }
        }
    } else if !createIfMissing {
        return 0, os.ErrNotExist
    }

    added := 0
    for _, l := range newLines {
        s := normalizePattern(l)
        if s == "" {
            continue
        }
        if _, ok := set[s]; !ok {
            set[s] = struct{}{}
            added++
        }
    }

    // write back
    f, err := os.Create(path)
    if err != nil {
        log.Printf("AddFileToList: failed to create %s: %v", path, err)
        return 0, err
    }
    defer f.Close()
    for k := range set {
        if _, err := f.WriteString(k + "\n"); err != nil {
            return added, err
        }
    }

    // reload lists
    if err := b.LoadAll(); err != nil {
        log.Printf("AddFileToList: reload failed: %v", err)
    }
    log.Printf("AddFileToList: appended %d entries to %s", added, listName)
    return added, nil
}

// ReplaceListFromURL downloads the file and replaces the named list entirely with the parsed domains.
func (b *BlocklistManager) ReplaceListFromURL(listName, url string) (int, error) {
    if listName == "" || url == "" {
        return 0, errors.New("missing list name or url")
    }
    client := &http.Client{Timeout: 15 * time.Second}
    resp, err := client.Get(url)
    if err != nil {
        log.Printf("ReplaceListFromURL: failed to GET %s: %v", url, err)
        return 0, err
    }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return 0, errors.New("failed to fetch file: " + resp.Status)
    }

    newLines, _ := readLines(resp.Body)
    path := filepath.Join(b.dir, listName+".txt")
    f, err := os.Create(path)
    if err != nil {
        log.Printf("ReplaceListFromURL: failed to create %s: %v", path, err)
        return 0, err
    }
    defer f.Close()
    written := 0
    for _, l := range newLines {
        if l == "" { continue }
        if _, err := f.WriteString(l + "\n"); err != nil {
            return written, err
        }
        written++
    }
    if err := b.LoadAll(); err != nil {
        log.Printf("ReplaceListFromURL: reload failed: %v", err)
    }
    log.Printf("ReplaceListFromURL: wrote %d entries to %s", written, listName)
    return written, nil
}

// AddItemsToList appends unique normalized items into the named list file.
// items may contain raw domains; normalization is applied. If createIfMissing is true,
// the list file is created when missing.
func (b *BlocklistManager) AddItemsToList(listName string, items []string, createIfMissing bool) (int, error) {
    if listName == "" {
        return 0, errors.New("missing list name")
    }
    path := filepath.Join(b.dir, listName+".txt")
    set := make(map[string]struct{})
    // read existing
    if f, err := os.Open(path); err == nil {
        old, _ := readLines(f)
        _ = f.Close()
        for _, l := range old {
            s := normalizePattern(l)
            if s != "" {
                set[s] = struct{}{}
            }
        }
    } else if !createIfMissing {
        return 0, os.ErrNotExist
    }
    added := 0
    for _, it := range items {
        // split on commas/spaces/newlines if the single item contains many
        parts := strings.FieldsFunc(it, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\r' || r == '\t' })
        for _, p := range parts {
            s := normalizePattern(p)
            if s == "" { continue }
            if _, ok := set[s]; !ok {
                set[s] = struct{}{}
                added++
            }
        }
    }
    // write back
    f, err := os.Create(path)
    if err != nil {
        return 0, err
    }
    defer f.Close()
    for k := range set {
        if _, err := f.WriteString(k + "\n"); err != nil {
            return added, err
        }
    }
    if err := b.LoadAll(); err != nil {
        log.Printf("AddItemsToList: reload failed: %v", err)
    }
    log.Printf("AddItemsToList: appended %d entries to %s", added, listName)
    return added, nil
}

// RecordQuery updates simple analytics counters. Call this from the DNS path for each query.
func (b *BlocklistManager) RecordQuery(domain string, blocked bool) {
    b.statsMu.Lock()
    b.queries++
    if blocked {
        b.blockedQueries++
        b.domainHits[domain]++
    }
    b.allHits[domain]++
    b.statsMu.Unlock()

    // append recent log (no client info)
    b.recentMu.Lock()
    defer b.recentMu.Unlock()
    entry := QueryEntry{Time: time.Now().UTC(), Domain: domain, Blocked: blocked}
    b.recent = append(b.recent, entry)
    if len(b.recent) > b.recentCap {
        drop := len(b.recent) - b.recentCap
        b.recent = b.recent[drop:]
    }
    // persist to disk (best-effort)
    go b.appendLog(entry)
}

// RecordQueryWithClient records a query including the client's address.
func (b *BlocklistManager) RecordQueryWithClient(domain, client string, blocked bool) {
    b.statsMu.Lock()
    b.queries++
    if blocked {
        b.blockedQueries++
        b.domainHits[domain]++
    }
    b.allHits[domain]++
    if client != "" {
        b.clientHits[client]++
    }
    b.statsMu.Unlock()

    b.recentMu.Lock()
    defer b.recentMu.Unlock()
    entry := QueryEntry{Time: time.Now().UTC(), Domain: domain, Client: client, Blocked: blocked}
    b.recent = append(b.recent, entry)
    if len(b.recent) > b.recentCap {
        drop := len(b.recent) - b.recentCap
        b.recent = b.recent[drop:]
    }
    // persist to disk (best-effort)
    go b.appendLog(entry)
}

// appendLog writes a single QueryEntry as a JSON line to the log file. Best-effort: failures are logged but not returned.
func (b *BlocklistManager) appendLog(e QueryEntry) {
    if b.logPath == "" {
        return
    }
    b.logMu.Lock()
    defer b.logMu.Unlock()
    f, err := os.OpenFile(b.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
    if err != nil {
        log.Printf("appendLog: open failed: %v", err)
        return
    }
    defer f.Close()
    data, err := json.Marshal(e)
    if err != nil {
        log.Printf("appendLog: marshal failed: %v", err)
        return
    }
    if _, err := f.Write(append(data, '\n')); err != nil {
        log.Printf("appendLog: write failed: %v", err)
        return
    }
}

// DeleteLogs truncates the persistent log file and clears in-memory recent logs.
func (b *BlocklistManager) DeleteLogs() error {
    b.logMu.Lock()
    defer b.logMu.Unlock()
    if b.logPath != "" {
        // truncate file
        if err := os.Truncate(b.logPath, 0); err != nil {
            // If the file doesn't exist, that's fine; attempt to create it
            if !os.IsNotExist(err) {
                return err
            }
            if f, err := os.Create(b.logPath); err == nil { _ = f.Close() }
        }
    }
    b.recentMu.Lock()
    b.recent = make([]QueryEntry, 0, b.recentCap)
    b.recentMu.Unlock()
    return nil
}

// GetLogs returns up to `limit` most recent QueryEntry records (most recent last).
func (b *BlocklistManager) GetLogs(limit int) []QueryEntry {
    b.recentMu.Lock()
    defer b.recentMu.Unlock()
    if limit <= 0 || limit > len(b.recent) {
        limit = len(b.recent)
    }
    res := make([]QueryEntry, limit)
    copy(res, b.recent[len(b.recent)-limit:])
    return res
}

// StatsSnapshot holds simple analytics data returned by the API.
type StatsSnapshot struct {
    Queries       int            `json:"queries"`
    Blocked       int            `json:"blocked"`
    DomainHits    map[string]int `json:"domain_hits"`
    ClientHits    map[string]int `json:"client_hits"`
}

// GetStats returns a snapshot of analytics.
func (b *BlocklistManager) GetStats() StatsSnapshot {
    b.statsMu.RLock()
    defer b.statsMu.RUnlock()
    // shallow copy map
    dh := make(map[string]int, len(b.domainHits))
    for k, v := range b.domainHits {
        dh[k] = v
    }
    ch := make(map[string]int, len(b.clientHits))
    for k, v := range b.clientHits {
        ch[k] = v
    }
    return StatsSnapshot{Queries: b.queries, Blocked: b.blockedQueries, DomainHits: dh, ClientHits: ch}
}

// ListDomains returns domains from a named list with simple pagination and optional substring search.
func (b *BlocklistManager) ListDomains(listName string, offset, limit int, q string) (total int, items []string, err error) {
    b.mu.RLock()
    arr, ok := b.lists[listName]
    b.mu.RUnlock()
    if !ok {
        return 0, nil, os.ErrNotExist
    }
    lowerQ := strings.ToLower(strings.TrimSpace(q))
    filtered := make([]string, 0, len(arr))
    if lowerQ == "" {
        filtered = append(filtered, arr...)
    } else {
        for _, d := range arr {
            if strings.Contains(d, lowerQ) {
                filtered = append(filtered, d)
            }
        }
    }
    total = len(filtered)
    if offset < 0 { offset = 0 }
    if limit <= 0 { limit = 100 }
    if offset >= total {
        return total, []string{}, nil
    }
    end := offset + limit
    if end > total { end = total }
    items = filtered[offset:end]
    return total, items, nil
}

// RemoveDomain removes a domain from the named list file and reloads lists.
func (b *BlocklistManager) RemoveDomain(listName, domain string) (bool, error) {
    if listName == "" || domain == "" {
        return false, errors.New("missing parameters")
    }
    b.mu.Lock()
    arr, ok := b.lists[listName]
    if !ok {
        b.mu.Unlock()
        return false, os.ErrNotExist
    }
    norm := normalizePattern(domain)
    newArr := make([]string, 0, len(arr))
    removed := false
    for _, d := range arr {
        if d == norm {
            removed = true
            continue
        }
        newArr = append(newArr, d)
    }
    b.mu.Unlock()
    if !removed {
        return false, nil
    }
    // write file
    path := filepath.Join(b.dir, listName+".txt")
    f, err := os.Create(path)
    if err != nil {
        return false, err
    }
    defer f.Close()
    for _, d := range newArr {
        if _, err := f.WriteString(d + "\n"); err != nil {
            return false, err
        }
    }
    if err := b.LoadAll(); err != nil {
        return true, err
    }
    return true, nil
}

// helper: parseHostsLines reads hosts-formatted content and returns a slice
// of domains found. It supports lines like:
//   0.0.0.0 domain.tld
//   127.0.0.1 domain.tld another.domain.tld
// It strips inline comments ("# ..."), ignores blank lines and comment lines,
// and filters out IP-only entries and common localhost names.
func readLines(r io.Reader) ([]string, error) {
    s := bufio.NewScanner(r)
    domains := make([]string, 0)
    for s.Scan() {
        line := s.Text()
        // strip inline comment
        if idx := strings.Index(line, "#"); idx >= 0 {
            line = line[:idx]
        }
        line = strings.TrimSpace(line)
        if line == "" {
            continue
        }
        fields := strings.Fields(line)
        if len(fields) == 0 {
            continue
        }
        // Accept both hosts-style lines (IP + hostnames) and plain domain-per-line.
        // If the first token looks like an IP, treat remaining tokens as hosts.
        // Otherwise treat all tokens as hostnames/domains.
        startIdx := 0
        if isIPString(fields[0]) {
            // hosts-style line: IP followed by one or more hostnames
            if len(fields) < 2 {
                continue
            }
            startIdx = 1
        }
        for i := startIdx; i < len(fields); i++ {
            h := strings.TrimSpace(fields[i])
            if h == "" {
                continue
            }
            n := normalizePattern(h)
            if n == "" || isLocalHostName(n) || isIPString(n) {
                continue
            }
            domains = append(domains, n)
        }
    }
    return domains, s.Err()
}

func isIPString(s string) bool {
    // net.ParseIP handles IPv4 and IPv6
    return net.ParseIP(s) != nil
}

func isLocalHostName(s string) bool {
    // common names in hosts files we don't want to treat as block targets
    switch s {
    case "localhost", "local", "broadcasthost", "ip6-localhost", "ip6-loopback":
        return true
    }
    // also ignore entries that start with "ip6-" or "ff" multicast markers used in sample
    if strings.HasPrefix(s, "ip6-") || strings.HasPrefix(s, "ff") {
        return true
    }
    return false
}

func normalizePattern(p string) string {
    p = strings.TrimSpace(p)
    p = strings.TrimSuffix(p, ".")
    p = strings.ToLower(p)
    if p == "" || strings.HasPrefix(p, "#") {
        return ""
    }
    return p
}

// patternToRegexp converts a wildcard pattern into a regexp that matches whole domain names.
// Rules:
//  - '*' matches any sequence of characters (including dots).
//  - patterns are matched against the full domain string (no trailing dot).
//  - example: "*.example.com" -> matches "sub.example.com" but not "example.com".
func patternToRegexp(p string) (*regexp.Regexp, error) {
    p = normalizePattern(p)
    if p == "" {
        return nil, nil
    }
    // Escape regex meta then replace escaped '*' with '.*'
    esc := regexp.QuoteMeta(p)
    esc = strings.ReplaceAll(esc, "\\*", ".*")
    full := "^" + esc + "$"
    return regexp.Compile(full)
}
