# Run with elevated PowerShell
# This script will add a portproxy rule forwarding port 53 to 5353 on localhost.
# Run as Administrator.

Write-Host "Forwarding UDP/TCP port 53 -> 127.0.0.1:5353"
netsh interface portproxy add v4tov4 listenaddress=0.0.0.0 listenport=53 connectaddress=127.0.0.1 connectport=5353
netsh interface portproxy show all

Write-Host "To remove the rule later run: netsh interface portproxy delete v4tov4 listenaddress=0.0.0.0 listenport=53"