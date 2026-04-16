#!/usr/bin/env bash
set -euo pipefail

# Generate a local CA, server certificate, and client certificate for mTLS.
# Usage:
#   ./gen-certs.sh 192.168.22.171 192.168.22.172
# Output directory:
#   ./certs

SERVER_IP="${1:-192.168.22.171}"
PEER_IP="${2:-192.168.22.172}"
OUT_DIR="$(cd "$(dirname "$0")/.." && pwd)/certs"

mkdir -p "$OUT_DIR"

CA_KEY="$OUT_DIR/ca.key"
CA_CERT="$OUT_DIR/ca.crt"
SERVER_KEY="$OUT_DIR/server.key"
SERVER_CSR="$OUT_DIR/server.csr"
SERVER_CERT="$OUT_DIR/server.crt"
CLIENT_KEY="$OUT_DIR/client.key"
CLIENT_CSR="$OUT_DIR/client.csr"
CLIENT_CERT="$OUT_DIR/client.crt"

openssl genrsa -out "$CA_KEY" 4096
openssl req -x509 -new -nodes -key "$CA_KEY" -sha256 -days 3650 \
  -subj "/C=VN/ST=HN/L=HN/O=SecurityLab/OU=PQC/CN=pqc-local-ca" \
  -out "$CA_CERT"

cat > "$OUT_DIR/server-ext.cnf" <<EOF
subjectAltName=DNS:ubuntu-server,IP:${SERVER_IP},IP:${PEER_IP},IP:127.0.0.1
extendedKeyUsage=serverAuth
keyUsage=digitalSignature,keyEncipherment
EOF

openssl genrsa -out "$SERVER_KEY" 2048
openssl req -new -key "$SERVER_KEY" \
  -subj "/C=VN/ST=HN/L=HN/O=SecurityLab/OU=PQC/CN=${SERVER_IP}" \
  -out "$SERVER_CSR"
openssl x509 -req -in "$SERVER_CSR" -CA "$CA_CERT" -CAkey "$CA_KEY" -CAcreateserial \
  -out "$SERVER_CERT" -days 825 -sha256 -extfile "$OUT_DIR/server-ext.cnf"

cat > "$OUT_DIR/client-ext.cnf" <<EOF
extendedKeyUsage=clientAuth
keyUsage=digitalSignature,keyEncipherment
EOF

openssl genrsa -out "$CLIENT_KEY" 2048
openssl req -new -key "$CLIENT_KEY" \
  -subj "/C=VN/ST=HN/L=HN/O=SecurityLab/OU=PQC/CN=pqc-client" \
  -out "$CLIENT_CSR"
openssl x509 -req -in "$CLIENT_CSR" -CA "$CA_CERT" -CAkey "$CA_KEY" -CAcreateserial \
  -out "$CLIENT_CERT" -days 825 -sha256 -extfile "$OUT_DIR/client-ext.cnf"

rm -f "$SERVER_CSR" "$CLIENT_CSR" "$OUT_DIR/server-ext.cnf" "$OUT_DIR/client-ext.cnf"

echo "Generated certificates in: $OUT_DIR"
echo "- CA:     $CA_CERT"
echo "- Server: $SERVER_CERT + $SERVER_KEY"
echo "- Client: $CLIENT_CERT + $CLIENT_KEY"
