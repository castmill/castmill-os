#!/usr/bin/env bash
# Create or renew the PRODUCTION signing keys of Castmill OS update bundles.
# Run it on your own computer, never on CI. See docs/release-signing.md.
#
#   scripts/gen-prod-keys.sh new DIR      root CA + first signing key
#   scripts/gen-prod-keys.sh renew DIR    new signing key from the root CA
#
# new writes into DIR (which must not exist yet):
#
#   root-ca.key.enc.pem   root CA key, encrypted with the printed passphrase.
#                         Keep it, with the passphrase, in your password
#                         manager and give a copy to a second trusted person.
#                         Never upload it anywhere.
#   root-ca.pem           root CA certificate (public): commit it as
#                         layers/meta-castmill/files/keys/production-keyring.pem
#   signing.key.pem       signing key: GitHub secret CASTMILL_PROD_RAUC_KEY
#   signing.cert.pem      signing certificate: GitHub secret CASTMILL_PROD_RAUC_CERT
#
# renew reads root-ca.key.enc.pem and root-ca.pem from DIR, asks for the
# passphrase and writes a new signing.key.pem / signing.cert.pem.
set -euo pipefail
umask 077

ROOT_DAYS=7300    # 20 years: devices trust this certificate for their lifetime
SIGNING_DAYS=730  # 2 years: renew the signing key before it expires
ORG="Castmill OS"

usage() {
  sed -n '2,24p' "$0" | sed 's/^# \{0,1\}//'
  exit 2
}

[[ $# -eq 2 ]] || usage
cmd="$1"
dir="$2"
command -v openssl >/dev/null || { echo "openssl not found" >&2; exit 1; }

cnf="$(mktemp)"
trap 'rm -f "$cnf" "${dir:-.}/signing.csr.pem" "${dir:-.}/root-ca.srl" "${dir:-.}/.passphrase"' EXIT
cat > "$cnf" <<'CNF'
[ req ]
distinguished_name = req_dn
prompt = no
[ req_dn ]
CN = unused
[ v3_ca ]
basicConstraints = critical,CA:true,pathlen:0
keyUsage = critical,keyCertSign,cRLSign
subjectKeyIdentifier = hash
[ v3_leaf ]
basicConstraints = critical,CA:false
keyUsage = critical,digitalSignature
extendedKeyUsage = codeSigning
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid
CNF

issue_signing() { # passphrase file
  # Written under temporary names and kept only if everything worked, so a
  # wrong passphrase never leaves a key without its certificate.
  local stamp
  stamp="$(date -u +%Y-%m)"
  rm -f "$dir/signing.key.new" "$dir/signing.cert.new"
  if ! {
    openssl ecparam -name prime256v1 -genkey -noout -out "$dir/signing.key.new" &&
      openssl req -new -key "$dir/signing.key.new" -config "$cnf" \
        -subj "/O=$ORG/CN=Castmill OS bundle signing $stamp" -out "$dir/signing.csr.pem" &&
      openssl x509 -req -in "$dir/signing.csr.pem" -CA "$dir/root-ca.pem" \
        -CAkey "$dir/root-ca.key.enc.pem" -passin "file:$1" -CAcreateserial -sha256 \
        -days "$SIGNING_DAYS" -extfile "$cnf" -extensions v3_leaf -out "$dir/signing.cert.new" &&
      openssl verify -CAfile "$dir/root-ca.pem" "$dir/signing.cert.new" >/dev/null
  }; then
    rm -f "$dir/signing.key.new" "$dir/signing.cert.new"
    echo "Could not issue the signing key (wrong passphrase?)." >&2
    return 1
  fi
  mv "$dir/signing.key.new" "$dir/signing.key.pem"
  mv "$dir/signing.cert.new" "$dir/signing.cert.pem"
}

case "$cmd" in
new)
  if [[ -e "$dir" ]]; then
    echo "$dir already exists: choose a new, empty directory." >&2
    exit 1
  fi
  mkdir -p "$dir"
  pass="$dir/.passphrase"
  openssl rand -base64 33 | tr -d '\n' > "$pass"
  # Root CA: P-384, its key only ever stored encrypted.
  openssl ecparam -name secp384r1 -genkey -noout |
    openssl pkcs8 -topk8 -v2 aes-256-cbc -passout "file:$pass" -out "$dir/root-ca.key.enc.pem"
  openssl req -x509 -new -key "$dir/root-ca.key.enc.pem" -passin "file:$pass" -sha384 \
    -days "$ROOT_DAYS" -config "$cnf" -extensions v3_ca \
    -subj "/O=$ORG/CN=Castmill OS bundle root CA" -out "$dir/root-ca.pem"
  issue_signing "$pass"
  passphrase="$(cat "$pass")"
  rm -f "$pass"
  cat <<EOF

Production keys written to $dir

1. Save in your password manager (and give your partner a copy):
     - the file  $dir/root-ca.key.enc.pem
     - its passphrase (shown once):

         $passphrase

2. Commit the root certificate (public):
     cp $dir/root-ca.pem layers/meta-castmill/files/keys/production-keyring.pem

3. Upload the signing key to the protected GitHub environment:
     gh secret set CASTMILL_PROD_RAUC_KEY  --env production-signing < $dir/signing.key.pem
     gh secret set CASTMILL_PROD_RAUC_CERT --env production-signing < $dir/signing.cert.pem

4. Delete $dir from this computer once 1-3 are done.
EOF
  ;;
renew)
  for f in root-ca.key.enc.pem root-ca.pem; do
    [[ -f "$dir/$f" ]] || { echo "$dir/$f not found (restore it from the password manager)." >&2; exit 1; }
  done
  if [[ -e "$dir/signing.key.pem" ]]; then
    echo "$dir/signing.key.pem exists: move it away first." >&2
    exit 1
  fi
  pass="$dir/.passphrase"
  read -r -s -p "Root CA passphrase: " passphrase
  echo
  printf '%s' "$passphrase" > "$pass"
  issue_signing "$pass"
  rm -f "$pass"
  cat <<EOF

New signing key written to $dir. Upload it:
  gh secret set CASTMILL_PROD_RAUC_KEY  --env production-signing < $dir/signing.key.pem
  gh secret set CASTMILL_PROD_RAUC_CERT --env production-signing < $dir/signing.cert.pem
Devices keep working: they trust the root CA, not a particular signing key.
Then delete $dir from this computer.
EOF
  ;;
*)
  usage
  ;;
esac
