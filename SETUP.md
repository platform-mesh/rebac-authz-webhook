# Platform Mesh Authorization Webhook Setup Guide

This guide explains how to set up the complete integration between rebac-authz-webhook, account-operator, fga-operator with OpenFGA and KCP for local development.

## Prerequisites

- Docker installed and running
- [Task](https://taskfile.dev/) installed (for running `task local-setup`)
- kubectl configured  
- Go 1.21+ installed

## Step 1: Setup Local Environment

1. **Adjust the values**:
In helm-charts-priv add coreModule to fga-operator values

```
coreModule: |
    module core

    type user

    type role
    relations
        define assignee: [user,user:*]

    type core_platform_mesh_io_account
    relations

        define parent: [account]
        define owner: [role#assignee]
        define member: [role#assignee] or owner

        define get: member or get from parent
        define update: member or update from parent
        define delete: owner or delete from parent

        define create_core_platform_mesh_io_accounts: member
        define list_core_platform_mesh_io_accounts: member
        define watch_core_platform_mesh_io_accounts: member

        define member_manage: owner or owner from parent

        # org and account specific
        define watch: member or watch from parent

        # org specific
        define create: member or create from parent
        define list: member or list from parent
```

In fga-operator replace account with core_platform_mesh_io_account

In account-operator replace account with core_platform_mesh_io_account


2. **Run the local setup task**:
   ```bash
   task local-setup
   ```

## Step 2: Setup KCP and AccountInfo

1. **Switch to KCP context**:
   ```bash
   export KUBECONFIG=/path/to/helm-charts-priv/.secret/kcp/admin.kubeconfig
   kubectl config use-context workspace.kcp.io/current
   ```

2. **Navigate to openmfp workspace**:
   ```bash
   kubectl ws use :root:orgs:openmfp
   ```

3. **Verify AccountInfo exists**:
   ```bash
   kubectl get accountinfos
   # Should show 'account' resource
   ```

4. **Get the cluster ID**:
   ```bash
   kubectl get accountinfo account -o jsonpath='{.metadata.annotations.kcp\.io/cluster}'
   ```
   Save this cluster ID for later use.

## Step 3: Fix KCP TLS Certificate Issues

1. **Create separate kubeconfig for webhook**:
   ```bash
   cp /path/to/helm-charts-priv/.secret/kcp/admin.kubeconfig /path/to/helm-charts-priv/.secret/kcp/webhook.kubeconfig
   ```

2. **Convert certificates to inline format**:
   ```bash
   cd /path/to/helm-charts-priv/.secret/kcp
   CLIENT_CERT_DATA=$(base64 -i client.crt)
   CLIENT_KEY_DATA=$(base64 -i client.key)
   ```

3. **Update webhook.kubeconfig**:
   Replace the client-certificate and client-key paths with client-certificate-data and client-key-data:
   ```yaml
   users:
   - name: kcp-admin
     user:
       client-certificate-data: <CLIENT_CERT_DATA>
       client-key-data: <CLIENT_KEY_DATA>
   ```

4. **Set correct context**:
   ```bash
   export KUBECONFIG=/path/to/helm-charts-priv/.secret/kcp/webhook.kubeconfig
   kubectl config use-context workspace.kcp.io/current
   ```

## Step 4: Setup Port Forwarding for OpenFGA

1. **Switch to kind context**:
   ```bash
   kubectl config use-context kind-openmfp
   ```

2. **Start gRPC port-forward** (Terminal 1):
   ```bash
   kubectl port-forward pod/$(kubectl get pods -n openmfp-system -l app.kubernetes.io/name=openfga -o name | head -1 | cut -d/ -f2) 8081:8081 -n openmfp-system
   ```

3. **Start HTTP port-forward** (Terminal 2):
   ```bash
   kubectl port-forward pod/$(kubectl get pods -n openmfp-system -l app.kubernetes.io/name=openfga -o name | head -1 | cut -d/ -f2) 8080:8080 -n openmfp-system
   ```

## Step 5: Create OpenFGA Authorization Model

1. **Create authorization model file**:
   ```bash
   cat > auth_model.json << 'EOF'
   {
     "schema_version": "1.1",
     "type_definitions": [
       {
         "type": "user",
         "relations": {},
         "metadata": {
           "relations": {},
           "module": "",
           "source_info": {"file": ""}
         }
       },
       {
         "type": "role",
         "relations": {
           "assignee": {
             "this": {}
           }
         },
         "metadata": {
           "relations": {
             "assignee": {
               "directly_related_user_types": [
                 {
                   "type": "user",
                   "condition": ""
                 }
               ],
               "module": "",
               "source_info": null
             }
           },
           "module": "",
           "source_info": {"file": ""}
         }
       },
       {
         "type": "core_platform_mesh_io_account",
         "relations": {
           "owner": {
             "this": {}
           },
           "member": {
             "union": {
               "child": [
                 {
                   "this": {}
                 },
                 {
                   "computedUserset": {
                     "object": "",
                     "relation": "owner"
                   }
                 }
               ]
             }
           },
           "get": {
             "computedUserset": {
               "object": "",
               "relation": "member"
             }
           },
           "update": {
             "computedUserset": {
               "object": "",
               "relation": "member"
             }
           },
           "delete": {
             "computedUserset": {
               "object": "",
               "relation": "owner"
             }
           },
           "list": {
             "computedUserset": {
               "object": "",
               "relation": "member"
             }
           },
           "list_core_platform_mesh_io_accounts": {
             "computedUserset": {
               "object": "",
               "relation": "member"
             }
           },
           "create": {
             "computedUserset": {
               "object": "",
               "relation": "member"
             }
           },
           "watch": {
             "computedUserset": {
               "object": "",
               "relation": "member"
             }
           }
         },
         "metadata": {
           "relations": {
             "owner": {
               "directly_related_user_types": [
                 {
                   "type": "role",
                   "relation": "assignee",
                   "condition": ""
                 }
               ],
               "module": "",
               "source_info": null
             },
             "member": {
               "directly_related_user_types": [
                 {
                   "type": "role",
                   "relation": "assignee",
                   "condition": ""
                 }
               ],
               "module": "",
               "source_info": null
             }
           },
           "module": "",
           "source_info": {"file": ""}
         }
       }
     ],
     "conditions": {}
   }
   EOF
   ```

2. **Get Store ID from AccountInfo**:
   ```bash
   export KUBECONFIG=/path/to/helm-charts-priv/.secret/kcp/webhook.kubeconfig
   kubectl get accountinfo account -o jsonpath='{.spec.fga.store.id}'
   ```
   Save this Store ID.

3. **Upload authorization model**:
   ```bash
   curl -X POST "http://localhost:8080/stores/YOUR_STORE_ID/authorization-models" \
     -H "Content-Type: application/json" \
     -d @auth_model.json
   ```

## Step 6: Create User Permissions

1. **Create tuples file**:
   ```bash
   cat > tuples.json << 'EOF'
   {
     "writes": {
       "tuple_keys": [
         {
           "object": "role:admin",
           "relation": "assignee",
           "user": "user:YOUR_EMAIL"
         },
         {
           "object": "core_platform_mesh_io_account:CLUSTER_ID/ACCOUNT_NAME",
           "relation": "member",
           "user": "role:admin#assignee"
         }
       ]
     }
   }
   EOF
   ```

   Replace:
   - `YOUR_EMAIL` with your email
   - `CLUSTER_ID` with the cluster ID from Step 2
   - `ACCOUNT_NAME` with account name from AccountInfo (usually `openmfp`)

2. **Create tuples**:
   ```bash
   curl -X POST "http://localhost:8080/stores/YOUR_STORE_ID/write" \
     -H "Content-Type: application/json" \
     -d @tuples.json
   ```

## Step 7: Configure Webhook

1. **Create .env file**:
   ```bash
   cat > .env << 'EOF'
   KCP_KUBECONFIG_PATH=/path/to/kcp/webhook.kubeconfig
   USE_IN_CLUSTER_CLIENT=true
   RULELESS_MODE=true
   OPENFGA_ADDR=localhost:8081
   KCP_CLUSTER_URL=https://kcp.api.portal.dev.local:8443
   EOF
   ```

2. **Create test request**:
   ```bash
   cat > test-request.json << 'EOF'
   {
     "apiVersion": "authorization.k8s.io/v1",
     "kind": "SubjectAccessReview",
     "spec": {
       "user": "YOUR_EMAIL",
       "resourceAttributes": {
         "verb": "list",
         "group": "core.platform-mesh.io",
         "version": "v1alpha1",
         "resource": "accounts"
       },
       "extra": {
         "authorization.kubernetes.io/cluster-name": ["CLUSTER_ID"]
       }
     }
   }
   EOF
   ```

## Step 8: Test the Setup

1. **Run webhook in debug mode** (Terminal 3 or run debug mode in IDE):
   ```bash
   go run main.go serve --health-probe-addr=:8082
   ```

2. **Test authorization** (Terminal 4):
   ```bash
   curl -k -X POST https://localhost:9443/authz \
     -H "Content-Type: application/json" \
     -d @test-request.json
   ```

3. **Expected successful response**:
   ```json
   {
     "apiVersion": "authorization.k8s.io/v1",
     "kind": "SubjectAccessReview",
     "status": {
       "allowed": true,
       "denied": false
     }
   }
   ```

## Troubleshooting

### Common Issues:

1. **TLS Certificate Error**: Make sure webhook.kubeconfig uses certificate-data instead of file paths
2. **Port Forward Issues**: Ensure you're using the correct namespace (`openmfp-system`) and pod names
3. **gRPC vs HTTP**: Webhook uses gRPC (port 8081), web interface uses HTTP (port 8080)
4. **Wrong Cluster ID**: Use the cluster ID from annotation, not the path
5. **Authorization Model**: Ensure the model includes `list_core_platform_mesh_io_accounts` relation

### Debug Commands:

- Check OpenFGA stores: `curl -s http://localhost:8080/stores`
- Check authorization models: `curl -s "http://localhost:8080/stores/YOUR_STORE_ID/authorization-models"`
- Find webhook namespace: `WEBHOOK_NS=$(kubectl get deployment -A | grep rebac-authz-webhook | awk '{print $1}')`
- Check webhook deployment: `kubectl describe deployment rebac-authz-webhook -n $WEBHOOK_NS`
- Check webhook service: `kubectl get svc -A | grep rebac-authz-webhook`

## Architecture Overview

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  Webhook        │    │      KCP        │    │    OpenFGA      │
│                 │    │                 │    │                 │
│ ┌─────────────┐ │    │ ┌─────────────┐ │    │ ┌─────────────┐ │
│ │   Handler   │◄┼────┼►│ AccountInfo │ │    │ │    Store    │ │
│ │             │ │    │ │             │ │    │ │             │ │
│ └─────────────┘ │    │ └─────────────┘ │    │ └─────────────┘ │
│        │        │    │                 │    │        ▲        │
│        │        │    └─────────────────┘    │        │        │
│        │        │                           │ ┌─────────────┐ │
│        └────────┼───────────────────────────┼►│ Auth Model  │ │
│                 │                           │ │   + Tuples  │ │
└─────────────────┘                           │ └─────────────┘ │
                                              └─────────────────┘
```

### Flow Description:

1. **User Request**: `kubectl` → `kcp`
2. **Authorization Check**: `kcp` sends `SubjectAccessReview` to `rebac-authz-webhook`
3. **AccountInfo Lookup**: Webhook retrieves `AccountInfo` and `StoreID` from `kcp`
4. **Permission Check**: Webhook generates contextual tuples and checks permissions in `OpenFGA`
