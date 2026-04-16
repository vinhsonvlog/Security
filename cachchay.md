# Cach build va chay nhanh (Windows 192.168.22.172 <-> Ubuntu 192.168.22.171)

## 1) Build binary Go

Chay tren may build tai thu muc `elastic-agent`:

```bash
cd /Users/sonnguyen/Desktop/Security/elastic-agent

# Build cho Ubuntu
GOOS=linux GOARCH=amd64 go build -o build/pqc-tls-gateway-linux ./examples/pqc-tls-gateway
GOOS=linux GOARCH=amd64 go build -o build/pqc-http-log-sender-linux ./examples/pqc-http-log-sender

# Build cho Windows
GOOS=windows GOARCH=amd64 go build -o build/pqc-tls-gateway-windows.exe ./examples/pqc-tls-gateway
GOOS=windows GOARCH=amd64 go build -o build/pqc-http-log-sender-windows.exe ./examples/pqc-http-log-sender
```

Copy file binary tuong ung sang tung may.

## 2) Tao cert cho 2 IP

Chay tren Ubuntu:

```bash
cd /Users/sonnguyen/Desktop/Security/elastic-agent/examples/pqc-crosshost/scripts
chmod +x gen-certs.sh
./gen-certs.sh 192.168.22.171 192.168.22.172
```

Sau khi tao xong, copy 3 file nay sang may gui (Windows):

- `ca.crt`
- `client.crt`
- `client.key`

Vi tri tren Ubuntu:
`/Users/sonnguyen/Desktop/Security/elastic-agent/examples/pqc-crosshost/certs`

## 3) Ubuntu chay Logstash receiver

```bash
docker run --rm --name logstash-pqc -p 8080:8080 \
	-v "/Users/sonnguyen/Desktop/Security/elastic-agent/examples/pqc-crosshost/logstash/pipeline:/usr/share/logstash/pipeline" \
	docker.elastic.co/logstash/logstash:8.19.0
```

## 4) Ubuntu chay PQC TLS gateway

Mo terminal moi tren Ubuntu:

```bash
cd /Users/sonnguyen/Desktop/Security/elastic-agent
./build/pqc-tls-gateway-linux \
	-listen :8443 \
	-upstream http://127.0.0.1:8080 \
	-cert ./examples/pqc-crosshost/certs/server.crt \
	-key ./examples/pqc-crosshost/certs/server.key \
	-require-pqc true \
	-normalize-ecs true
```

## 5) Windows gui log sang Ubuntu

PowerShell tren Windows:

```powershell
cd C:\path\to\elastic-agent
.\build\pqc-http-log-sender-windows.exe `
	-endpoint "https://192.168.22.171:8443/logs-pqc" `
	-root-ca "C:\pqc-certs\ca.crt" `
	-client-cert "C:\pqc-certs\client.crt" `
	-client-key "C:\pqc-certs\client.key" `
	-log '{"@timestamp":"2026-04-16T00:00:00Z","level":"info","message":"win -> ubuntu pqc tls13"}'
```

Neu thanh cong:

- Client in: `Log sent successfully`
- Terminal Logstash tren Ubuntu hien log JSON.

## 6) Test nguoc Ubuntu -> Windows

Lam tuong tu stack receiver tren Windows (Logstash + pqc-tls-gateway),
sau do tu Ubuntu dung `pqc-http-log-sender-linux` gui den `https://192.168.22.172:8443/logs-pqc`.

## 7) Ghi chu

- Bat buoc Go phai ho tro `tls.X25519MLKEM768`.
- Gateway dang ep TLS 1.3 + nhom lai PQC `x25519mlkem768`.
- Nho mo firewall port 8443 va 8080 tren may nhan.
