#!/bin/bash

# 生成自签名SSL证书脚本
# 用于开发/测试环境

set -e

CERT_DIR="./certs"
DOMAIN="${1:-localhost}"

echo "生成自签名SSL证书..."
echo "域名: $DOMAIN"
echo "证书目录: $CERT_DIR"

# 创建证书目录
mkdir -p "$CERT_DIR"

# 生成私钥
echo "1. 生成私钥..."
openssl genrsa -out "$CERT_DIR/server.key" 2048

# 生成证书签名请求配置文件
cat > "$CERT_DIR/server.conf" <<EOF
[req]
default_bits = 2048
prompt = no
default_md = sha256
distinguished_name = dn
req_extensions = v3_req

[dn]
C=CN
ST=State
L=City
O=Organization
CN=$DOMAIN

[v3_req]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
subjectAltName = @alt_names

[alt_names]
DNS.1 = $DOMAIN
DNS.2 = *.$DOMAIN
IP.1 = 127.0.0.1
IP.2 = ::1
EOF

# 生成证书签名请求
echo "2. 生成证书签名请求..."
openssl req -new -key "$CERT_DIR/server.key" -out "$CERT_DIR/server.csr" -config "$CERT_DIR/server.conf"

# 生成自签名证书（有效期365天）
echo "3. 生成自签名证书..."
openssl x509 -req -days 365 -in "$CERT_DIR/server.csr" \
    -signkey "$CERT_DIR/server.key" \
    -out "$CERT_DIR/server.crt" \
    -extensions v3_req \
    -extfile "$CERT_DIR/server.conf"

# 清理临时文件
rm -f "$CERT_DIR/server.csr" "$CERT_DIR/server.conf"

echo ""
echo "✅ 证书生成成功！"
echo ""
echo "证书文件:"
echo "  - 证书: $CERT_DIR/server.crt"
echo "  - 私钥: $CERT_DIR/server.key"
echo ""
echo "配置示例 (config/config.yaml):"
echo "server:"
echo "  port: 8866"
echo "  enable_https: true"
echo "  cert_file: ./certs/server.crt"
echo "  key_file: ./certs/server.key"
echo ""
echo "⚠️  注意: 这是自签名证书，浏览器会显示安全警告，仅用于开发/测试环境！"
