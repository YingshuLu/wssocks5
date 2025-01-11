# wssocks5
Socks5 over WebSocket

## Inner Subnet
```./wssocks5 --mode client --listenport 8778 --serverurl wss://localhost:8443/socks5 --secret mytoken --clientcount 9 ```

## Public Net
```./wssocks5 --mode server --serverurl wss://localhost:8443/socks5 --secret mytoken```