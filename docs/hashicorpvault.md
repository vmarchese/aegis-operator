# Hashicorp Vault setup

1. Enable the jwt auth method:
```
vault auth enable jwt
```

2. federate the kubernetes cluster:
```
vault write auth/jwt/config \
    oidc_discovery_url=<KUBERNETES URL> \
    oidc_discovery_ca_pem=@<KUBERNETES CA PEM FILE> \
    bound_issuer=<KUBERNETES ISSUER>
```

The Kubernetes CA certificate PEM can be obtained with:
```
kubectl get cm kube-root-ca.crt \
    -o jsonpath="{['data']['ca\.crt']}" > kubernetes_ca.crt
```


3. Create the `aegis-operator` policy in Hashicorp Vault
```
path "identity/entity" {
    capabilities = ["create", "read", "update", "delete"]
}
path "identity/entity/name/*" {
    capabilities = ["read","delete"]
}
path "identity/oidc/role/*" {
    capabilities = ["create", "read", "update", "delete"]
}
path "identity/entity-alias" {
    capabilities = ["create", "read", "update","delete"]
}

path "identity/entity-alias/id/*" {
    capabilities = [ "read","delete"]
}
path "auth/jwt/role/*"{
    capabilities = ["create", "read", "update","delete"]
}

path "sys/auth" {
    capabilities = ["read"]
}
```

4. Create the jwt AEGIS role 
```
vault write auth/jwt/role/aegis \
   role_type="jwt" \
   bound_audiences="vault" \
   user_claim="sub" \
   bound_subject="system:serviceaccount:operator-system:operator-controller-manager" \
   policies="aegis-operator" \
   ttl="1h"
```   

5. Create the `jwt_issuer` policy
```
path "identity/oidc/token/*" {capabilities = ["create","read"]}
```


6. Create a named key 
```
POST {{vault_address}}/v1/identity/oidc/key/aegis-key
X-Vault-Token: {{vault_token}}

{
    "name": "aegis-key",
    "allowed_client_ids": ["*"]
}
```


