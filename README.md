# wssocks5
Socks5 over WebSocket

## Private Net
```./wssocks5 --mode client --listenport 8778 --serverurl wss://{server}:8443/socks5 --secret mytoken --clientcount 9 ```

## Public Net
```./wssocks5 --mode server --serverurl wss://{server}:8443/socks5 --secret mytoken```
