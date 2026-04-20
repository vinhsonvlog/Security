# PQC TLS 1.3 lab: Windows <-> Ubuntu with Logstash receiver

Muc tieu ngan gon:

- Su dung Go de gui log qua ket noi TLS 1.3 + nhom lai PQC `x25519mlkem768`.
- Ubuntu (`192.168.22.171`) dong vai tro server nhan log qua Logstash.
- Kiem thu duoc theo huong Windows -> Ubuntu truoc, sau do dao chieu Ubuntu -> Windows.

## 1) Thanh phan su dung

Tai repo `elastic-agent` da co san:

- `examples/pqc-tls-gateway/main.go`: reverse proxy TLS 1.3, ep `x25519mlkem768`, forward vao upstream HTTP.
- `examples/pqc-http-log-sender/main.go`: Go sender gui JSON log qua HTTPS voi `x25519mlkem768`.

Trong thu muc nay bo sung:

- `scripts/gen-certs.sh`: tao CA + cert server/client (mTLS).
- `logstash/pipeline/pqc-http.conf`: pipeline Logstash nhan HTTP JSON va in ra stdout.

## 2) Build nhanh binary Go

Chay tai may build (Linux hoac macOS):

```bash
cd elastic-agent

# Linux (Ubuntu server)
GOOS=linux GOARCH=amd64 go build -o build/pqc-tls-gateway-linux ./examples/pqc-tls-gateway
GOOS=linux GOARCH=amd64 go build -o build/pqc-http-log-sender-linux ./examples/pqc-http-log-sender

# Windows
GOOS=windows GOARCH=amd64 go build -o build/pqc-tls-gateway-windows.exe ./examples/pqc-tls-gateway
GOOS=windows GOARCH=amd64 go build -o build/pqc-http-log-sender-windows.exe ./examples/pqc-http-log-sender
```

Copy binary tuong ung sang tung may.

## 3) Tao cert dung cho 2 may

Chay tren Ubuntu server:

```bash
cd elastic-agent/examples/pqc-crosshost/scripts
chmod +x gen-certs.sh
./gen-certs.sh 192.168.22.171 192.168.22.172
```

Cert nam o `elastic-agent/examples/pqc-crosshost/certs`.
Can copy `ca.crt`, `client.crt`, `client.key` sang may gui.

## 4) Ubuntu: chay Logstash receiver

Co 2 cach:

### Cach A - Docker nhanh

```bash
docker run --rm --name logstash-pqc -p 8080:8080 \
  -v "$PWD/elastic-agent/examples/pqc-crosshost/logstash/pipeline:/usr/share/logstash/pipeline" \
  docker.elastic.co/logstash/logstash:8.19.0
```

### Cach B - Logstash local

Them file pipeline `pqc-http.conf` vao pipeline cua Logstash, mo cong 8080.

## 5) Ubuntu: chay PQC TLS reverse proxy truoc Logstash

```bash
cd elastic-agent
./build/pqc-tls-gateway-linux \
  -listen :8443 \
  -upstream http://127.0.0.1:8080 \
  -cert ./examples/pqc-crosshost/certs/server.crt \
  -key ./examples/pqc-crosshost/certs/server.key \
  -require-pqc true \
  -normalize-ecs true
```

Luc nay flow la:
Windows sender -> TLS 1.3 + x25519mlkem768 -> Go gateway (giai ma TLS) -> Logstash HTTP input -> stdout.

## 6) Windows gui log sang Ubuntu

Tren Windows (PowerShell), voi file cert da copy sang thu muc `C:\pqc-certs`:

```powershell
cd C:\path\to\elastic-agent
.\build\pqc-http-log-sender-windows.exe `
  -endpoint "https://192.168.22.171:8443/logs-pqc" `
  -root-ca "C:\pqc-certs\ca.crt" `
  -client-cert "C:\pqc-certs\client.crt" `
  -client-key "C:\pqc-certs\client.key" `
  -log '{"@timestamp":"2026-04-16T00:00:00Z","level":"info","message":"win -> ubuntu pqc tls13"}'
```

Neu thanh cong se thay `Log sent successfully` tren client va log JSON tren stdout cua Logstash.

## 6.1) Neu dang dung script Fleet sinh ra (.ps1/.bat) thi sua o dau?

Quan trong: script Fleet sinh ra chu yeu de **enroll Elastic Agent vao Fleet Server** (control plane).
No khong tu dong bien kenh data plane thanh PQC cho output Logstash.

Can tach 2 viec:

1. Enroll Agent -> Fleet Server qua HTTPS co CA xac thuc.
2. Gui log -> Logstash qua `pqc-tls-gateway` de ep TLS 1.3 + `x25519mlkem768`.

### A) Sua lenh install trong script Fleet-generated

Vi du script goc (PowerShell) cua ban:

```powershell
.\elastic-agent.exe install --url=https://192.168.22.171:8220 --enrollment-token=<token>
```

Sua thanh (them CA, duong dan tuyet doi):

```powershell
.\elastic-agent.exe install `
  --url=https://192.168.22.171:8220 `
  --enrollment-token=<token> `
  --certificate-authorities="C:\pqc-certs\ca.crt"
```

Luu y:

- `--certificate-authorities` bat buoc dung duong dan tuyet doi.
- Neu chung chi Fleet Server do CA khac ky, thay `ca.crt` bang dung CA do.

### B) Dat file `ca.crt` va `ca.key` o dau?

- Tren may Agent (Windows): chi can `ca.crt` (vi du `C:\pqc-certs\ca.crt`).
- `ca.key` la private key cua CA: **khong copy len Agent**, chi giu o may CA/server de ky cert.

### C) Chinh kenh gui log sang Logstash de co PQC

Trong Fleet policy (output Logstash hoac output trung gian), host phai tro vao gateway:

- `https://192.168.22.171:8443`

Gateway (`examples/pqc-tls-gateway/main.go`) se ep TLS 1.3 + `x25519mlkem768`,
sau do moi forward ve Logstash HTTP input noi bo (vi du `http://127.0.0.1:8080`).

Neu chi sua lenh install/enroll ma khong doi output host qua gateway thi log van khong di qua lop ep PQC.

## 7) Kiem thu dao chieu Ubuntu -> Windows

De test qua lai nhanh, chay y chang stack receiver tren Windows (Logstash + pqc-tls-gateway), doi endpoint sang IP Windows `192.168.22.172`, roi dung sender Linux gui nguoc lai.

## 8) Huong custom de trien khai that voi Elastic Agent + reverse proxy

Kien truc khuyen nghi:

1. Elastic Agent tren endpoint gui log noi bo den local collector.
2. Collector/gui custom (hoac output trung gian) day log qua `pqc-tls-gateway` bang HTTPS.
3. `pqc-tls-gateway` tren server bat buoc TLS 1.3 + `x25519mlkem768`, xac thuc mTLS, sau do forward vao Logstash.
4. Logstash tiep tuc route sang Elasticsearch.

Loi ich:

- Reverse proxy Go la diem ep chinh sach PQC ro rang nhat.
- Tach biet policy TLS/PQC khoi pipeline xu ly log.
- De mo rong sau nay cho rate-limit, auth, retry, queue.

## 9) Luu y quan trong

- Can Go ban ho tro `tls.X25519MLKEM768` (Go moi).
- Neu tay client/server khong ho tro nhom lai nay, ket noi se bi tu choi khi `-require-pqc=true`.
- Mo firewall cho port 8443 (gateway) va 8080 (Logstash noi bo/thu nghiem).
- Trong moi truong production, dung cert PKI that, khong dung cert tu ky.
