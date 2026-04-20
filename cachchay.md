# Cach build va chay nhanh (Windows 192.168.22.172 <-> Ubuntu 192.168.22.171)

Ban nay la ban full de copy/paste truc tiep, gom ca:

- Luong test PQC TLS 1.3: Windows -> Ubuntu (Logstash)
- Cach sua script Fleet-generated (.ps1/.bat)
- Vi tri dat `ca.crt` va cach xu ly `ca.key`

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

Checklist copy binary:

- Ubuntu can: `build/pqc-tls-gateway-linux`
- Windows can: `build/pqc-http-log-sender-windows.exe`
- Neu test nguoc thi copy them:
  - Windows: `build/pqc-tls-gateway-windows.exe`
  - Ubuntu: `build/pqc-http-log-sender-linux`

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

Luu y bao mat quan trong:

- Tren may Agent/Windows chi can `ca.crt` (va neu mTLS thi them `client.crt`, `client.key`).
- `ca.key` la private key cua CA, KHONG copy len Agent. Chi giu o may CA/server de ky cert.

## 3) Ubuntu chay Logstash receiver

```bash
docker run --rm --name logstash-pqc -p 8080:8080 \
	-v "/Users/sonnguyen/Desktop/Security/elastic-agent/examples/pqc-crosshost/logstash/pipeline:/usr/share/logstash/pipeline" \
	docker.elastic.co/logstash/logstash:8.19.0
```

Giu terminal nay de quan sat log den.

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

Gateway se ep TLS 1.3 + nhom lai PQC `x25519mlkem768`, sau do forward ve Logstash `127.0.0.1:8080`.

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

Neu loi TLS/cert:

- Kiem tra endpoint dung IP trong SAN da tao o buoc 2.
- Kiem tra `-root-ca` tro dung file `ca.crt`.
- Kiem tra dong ho 2 may khong lech qua nhieu.

## 5.1) Neu dang dung script Fleet sinh ra thi sua de copy

Script Fleet-generated thuong dang nhu sau:

```powershell
$ProgressPreference = 'SilentlyContinue'
Invoke-WebRequest -Uri https://artifacts.elastic.co/downloads/beats/elastic-agent/elastic-agent-9.3.3+build202604082258-windows-x86_64.zip -OutFile elastic-agent-9.3.3+build202604082258-windows-x86_64.zip
Expand-Archive .\elastic-agent-9.3.3+build202604082258-windows-x86_64.zip -DestinationPath .
cd elastic-agent-9.3.3+build202604082258-windows-x86_64
.\elastic-agent.exe install --url=https://192.168.22.171:8220 --enrollment-token=<token>
```

Hay sua dong install thanh:

```powershell
.\elastic-agent.exe install `
	--url=https://192.168.22.171:8220 `
	--enrollment-token=<token> `
	--certificate-authorities="C:\pqc-certs\ca.crt"
```

Quan trong:

- `--certificate-authorities` chi anh huong kenh enroll Agent -> Fleet Server (control plane).
- De log di qua TLS 1.3 + PQC, output log phai day qua gateway `https://192.168.22.171:8443` (data plane).

## 5.2) Mau full script PowerShell de copy (fleet install + test sender)

```powershell
$ProgressPreference = 'SilentlyContinue'

$zip = 'elastic-agent-9.3.3+build202604082258-windows-x86_64.zip'
$dir = 'elastic-agent-9.3.3+build202604082258-windows-x86_64'
$fleetUrl = 'https://192.168.22.171:8220'
$enrollToken = '<token>'
$caPath = 'C:\pqc-certs\ca.crt'

Invoke-WebRequest -Uri "https://artifacts.elastic.co/downloads/beats/elastic-agent/$zip" -OutFile $zip
Expand-Archive ".\\$zip" -DestinationPath . -Force
Set-Location ".\\$dir"

.\elastic-agent.exe install `
	--url=$fleetUrl `
	--enrollment-token=$enrollToken `
	--certificate-authorities=$caPath

# Test gui 1 log qua PQC gateway
.\build\pqc-http-log-sender-windows.exe `
	-endpoint "https://192.168.22.171:8443/logs-pqc" `
	-root-ca "C:\pqc-certs\ca.crt" `
	-client-cert "C:\pqc-certs\client.crt" `
	-client-key "C:\pqc-certs\client.key" `
	-log '{"@timestamp":"2026-04-16T00:00:00Z","level":"info","message":"win -> ubuntu pqc tls13"}'
```

## 6) Test nguoc Ubuntu -> Windows

Lam tuong tu stack receiver tren Windows (Logstash + pqc-tls-gateway),
sau do tu Ubuntu dung `pqc-http-log-sender-linux` gui den `https://192.168.22.172:8443/logs-pqc`.

## 7) Ghi chu

- Bat buoc Go phai ho tro `tls.X25519MLKEM768`.
- Gateway dang ep TLS 1.3 + nhom lai PQC `x25519mlkem768`.
- Nho mo firewall port 8443 va 8080 tren may nhan.

## 8) Full quick run (tom tat 6 lenh)

1. Build binary (buoc 1).
2. Tao cert (buoc 2).
3. Ubuntu chay Logstash (buoc 3).
4. Ubuntu chay gateway PQC (buoc 4).
5. Windows enroll Fleet voi `--certificate-authorities` (buoc 5.1).
6. Windows gui log test qua `https://192.168.22.171:8443/logs-pqc` (buoc 5).
