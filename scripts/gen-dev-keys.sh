#!/usr/bin/env bash
# Create DEVELOPMENT signing keys for RAUC bundles.
#
#   scripts/gen-dev-keys.sh [DIR]     (default: build/castmill-dev-keys)
#
# Writes:
#   rauc-keyring.pem        CA certificate, installed on devices (keyring)
#   rauc-signing.cert.pem   signing certificate (issued by the CA)
#   rauc-signing.key.pem    signing key
#   ca.key.pem              CA key (keep it to issue new signing certs)
#
# Development keys only. Production keys live in an HSM/KMS and never touch
# CI runners or developer machines (spec 03 "Signing and keys"). Images built
# with these keys must never be shipped.
set -euo pipefail

DIR="${1:-build/castmill-dev-keys}"
if [[ -e "$DIR/rauc-keyring.pem" ]]; then
  echo "Keys already exist in $DIR (delete the directory to start over)." >&2
  exit 0
fi
command -v openssl >/dev/null || { echo "openssl not found" >&2; exit 1; }
mkdir -p "$DIR"
chmod 700 "$DIR"
cd "$DIR"

cat > openssl.cnf <<'CNF'
[ req ]
distinguished_name = req_dn
prompt = no
[ req_dn ]
CN = unused
[ v3_ca ]
basicConstraints = critical,CA:true
keyUsage = critical,keyCertSign,cRLSign
subjectKeyIdentifier = hash
[ v3_leaf ]
basicConstraints = critical,CA:false
keyUsage = critical,digitalSignature
extendedKeyUsage = codeSigning
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid
CNF

openssl ecparam -name prime256v1 -genkey -noout -out ca.key.pem
openssl req -x509 -new -key ca.key.pem -sha256 -days 3650 -config openssl.cnf -extensions v3_ca \
  -subj "/O=Castmill OS development/CN=Castmill OS development bundle CA" -out rauc-keyring.pem

openssl ecparam -name prime256v1 -genkey -noout -out rauc-signing.key.pem
openssl req -new -key rauc-signing.key.pem -subj "/O=Castmill OS development/CN=Castmill OS development signing" \
  -config openssl.cnf -out signing.csr.pem
openssl x509 -req -in signing.csr.pem -CA rauc-keyring.pem -CAkey ca.key.pem -CAcreateserial -sha256 -days 1825 \
  -extfile openssl.cnf -extensions v3_leaf -out rauc-signing.cert.pem
rm -f signing.csr.pem rauc-keyring.srl openssl.cnf
chmod 600 ./*.key.pem

openssl verify -CAfile rauc-keyring.pem rauc-signing.cert.pem
echo "Development keys written to $(pwd)"
