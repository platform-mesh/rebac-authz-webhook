#!/bin/bash

openssl genrsa -out ca.key 2048

openssl req -new -x509 -days 365 -key ca.key \
  -subj "/C=DE/CN=authz-server" -config openssl.conf \
  -out ca.crt

openssl req -newkey rsa:2048 -nodes -keyout tls.key \
  -subj "/C=DE/CN=authz-server" \
  -out tls.csr

# -extfile <(printf "subjectAltName=DNS:host.containers.internal") \
openssl x509 -req \
  -days 365 \
  -extfile <(printf "subjectAltName=DNS:localhost") \
  -in tls.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out tls.crt

# Generate client key and CSR
openssl genrsa -out client.key 2048
openssl req -new -key client.key \
  -subj "/C=DE/CN=authz-client" \
  -out client.csr

# Sign client certificate
openssl x509 -req \
  -days 365 \
  -in client.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out client.crt

rm *.csr


