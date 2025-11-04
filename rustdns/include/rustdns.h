// C header for rustdns FFI
#ifndef RUSTDNS_H
#define RUSTDNS_H

#ifdef __cplusplus
extern "C" {
#endif

int rustdns_start(const char* http_addr, const char* udp_bind);
int rustdns_stop();

#ifdef __cplusplus
}
#endif

#endif // RUSTDNS_H
