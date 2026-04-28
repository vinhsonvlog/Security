# Cach build va chay: Elastic Agent -> Proxy -> TLS1.3+PQC Gateway -> Logstash

Muc tieu:

- Ep duong gui log di qua TLS 1.3 + PQC (`x25519mlkem768`).
- Them proxy de Elastic Agent gui log ra ngoai duoc.
- Co lenh chay nhanh de copy/paste.

Luu y quan trong (doc truoc):

- Trong Elastic Agent config hien tai, ban co the ep `ssl.supported_protocols: [TLSv1.3]`.
- Nhung khong co field chinh thong de "khoa cung" `X25519MLKEM768` tren output.
- Vi vay diem ep PQC phai dat o `pqc-tls-gateway` voi `-require-pqc true`.
- Neu client khong negotiate `x25519mlkem768`, gateway se tu choi ket noi.

---

## 1) Build binary tren may build

```bash
cd /Users/sonnguyen/Desktop/Security/elastic-agent

# Linux
GOOS=linux GOARCH=amd64 go build -o build/pqc-tls-gateway-linux ./examples/pqc-tls-gateway
GOOS=linux GOARCH=amd64 go build -o build/pqc-http-log-sender-linux ./examples/pqc-http-log-sender

# Windows
GOOS=windows GOARCH=amd64 go build -o build/pqc-tls-gateway-windows.exe ./examples/pqc-tls-gateway
GOOS=windows GOARCH=amd64 go build -o build/pqc-http-log-sender-windows.exe ./examples/pqc-http-log-sender
```

Copy file:

- Ubuntu server: `build/pqc-tls-gateway-linux`
- Windows agent: `build/pqc-http-log-sender-windows.exe` (de test nhanh)

---

## 2) Tao cert cho server/client

```bash
cd /Users/sonnguyen/Desktop/Security/elastic-agent/examples/pqc-crosshost/scripts
chmod +x gen-certs.sh
./gen-certs.sh 192.168.22.171 192.168.22.172
```

Cert tao ra o:
`/Users/sonnguyen/Desktop/Security/elastic-agent/examples/pqc-crosshost/certs`

Copy sang Windows:

- `ca.crt`
- `client.crt`
- `client.key`

Khong copy `ca.key` len agent.

---

## 3) Ubuntu chay Logstash receiver

```bash
docker run --rm --name logstash-pqc -p 8080:8080 \
  -v "/Users/sonnguyen/Desktop/Security/elastic-agent/examples/pqc-crosshost/logstash/pipeline:/usr/share/logstash/pipeline" \
  docker.elastic.co/logstash/logstash:8.19.0
```

---

## 4) Ubuntu chay PQC TLS gateway (diem ep PQC)

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

Gateway nay bat buoc:

- TLS 1.3
- curve `x25519mlkem768` (neu `-require-pqc true`)

---

## 5) (Khuyen nghi) Chay proxy de test duong Elastic Agent ra ngoai

Neu ban chua co proxy doanh nghiep, dung Squid local:

```bash
docker run --rm --name squid -p 3128:3128 ubuntu/squid:latest
```

Gia su proxy IP la `192.168.22.171:3128`.
Proxy phai ho tro HTTP CONNECT cho dich vu TLS (`:8443`).

---

## 6) Fleet-managed: enroll Agent co proxy (control plane)

PowerShell tren Windows (script Fleet sua lai):

```powershell
.\elastic-agent.exe install `
  --url=https://192.168.22.171:8220 `
  --enrollment-token=<token> `
  --certificate-authorities="C:\pqc-certs\ca.crt" `
  --proxy-url="http://192.168.22.171:3128"
```

Neu proxy can header CONNECT:

```powershell
.\elastic-agent.exe install `
  --url=https://192.168.22.171:8220 `
  --enrollment-token=<token> `
  --certificate-authorities="C:\pqc-certs\ca.crt" `
  --proxy-url="http://192.168.22.171:3128" `
  --proxy-header "Proxy-Authorization=Basic <base64-user-pass>"
```

Luu y:

- `--proxy-url` tren lenh install/enroll chu yeu cho kenh Agent <-> Fleet Server.

---

## 7) Fleet-managed: output log qua gateway + proxy (data plane)

Trong Fleet policy output (Logstash output) dat:

- Host output tro vao gateway: `192.168.22.171:8443`
- Advanced YAML:

```yaml
proxy_url: "http://192.168.22.171:3128"
ssl.enabled: true
ssl.certificate_authorities: ["C:\\pqc-certs\\ca.crt"]
ssl.supported_protocols: ["TLSv1.3"]
```

Neu proxy khong can cho dich vu nay:

```yaml
proxy_disable: true
```

Quan trong:

- TLS1.3 duoc ep o output.
- PQC duoc ep boi `pqc-tls-gateway`, khong phai field output cua Elastic Agent.

---

## 8) Standalone mode (neu khong dung Fleet)

Mau `elastic-agent.yml`:

```yaml
outputs:
  default:
    type: logstash
    hosts: ["192.168.22.171:8443"]
    proxy_url: "http://192.168.22.171:3128"
    ssl:
      enabled: true
      certificate_authorities: ["C:/pqc-certs/ca.crt"]
      supported_protocols: ["TLSv1.3"]

inputs:
  - type: filestream
    id: test-filestream
    use_output: default
    streams:
      - id: test-filestream-1
        data_stream:
          dataset: custom.pqc
        paths:
          - C:/temp/test.log
```

---

## 9) Kiem tra nhanh

1. Test sender Go qua gateway (xem duong TLS/PQC):

```powershell
.\build\pqc-http-log-sender-windows.exe `
  -endpoint "https://192.168.22.171:8443/logs-pqc" `
  -proxy-url "http://192.168.22.171:3128" `
  -proxy-headers "Proxy-Authorization=Basic <base64-user-pass>" `
  -root-ca "C:\pqc-certs\ca.crt" `
  -client-cert "C:\pqc-certs\client.crt" `
  -client-key "C:\pqc-certs\client.key" `
  -log '{"@timestamp":"2026-04-28T00:00:00Z","level":"info","message":"agent path via proxy+pqc"}'
```

`-proxy-headers` la tuy chon, bo di neu proxy khong yeu cau auth/header.

2. Kiem tra Agent:

```powershell
elastic-agent status
elastic-agent inspect output -o default
```

3. Kiem tra terminal Logstash phai thay event den.

---

## 10) Troubleshoot nhanh

- Loi cert/SAN: xac nhan IP trong cert trung endpoint.
- Log khong ra ngoai qua proxy: kiem tra proxy cho phep CONNECT toi `192.168.22.171:8443`.
- Fleet enroll OK nhung data khong di: kiem tra lai output host trong policy da tro vao gateway PQC chua.
- Neu gateway bat `-require-pqc true` ma client khong ho tro `x25519mlkem768` thi se bi reject (dung theo thiet ke).
