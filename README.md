# proxmox-mcp

An [MCP](https://modelcontextprotocol.io) server that exposes [Proxmox VE](https://www.proxmox.com/en/proxmox-virtual-environment/) cluster operations as tools, built in Go using the official [go-sdk](https://github.com/modelcontextprotocol/go-sdk). Tool responses are compact JSON (no indentation) to minimise token usage.

> **This fork is tailored for Hermes Agent.** It keeps the upstream project’s general-purpose Proxmox MCP functionality while adding security and usability features for a Hermes deployment that is intentionally restricted to a Proxmox resource pool. The server remains usable by other MCP clients; Hermes is the deployment this fork was designed and tested around.
>
> Names such as `hermes@pve`, `hermes@pve!agent`, `HermesManaged`, `HermesVMAdmin`, and `PROXMOX_ALLOWED_POOL=HermesManaged` are conventional examples, not hard-coded requirements. Choose your own user, token, role, and pool names. `PROXMOX_ALLOWED_POOL` must match the pool configured for the agent.

The intended deployment shape is:

```text
Hermes Agent
     │
     │ MCP
     ▼
proxmox-mcp
     │
     │ dedicated privilege-separated API token
     ▼
Proxmox VE
     │
     ├── HermesManaged pool  ← agent may manage these VMs
     ├── Hermes VM           ← protected
     ├── Home Assistant VM   ← protected
     └── other resources     ← protected
```

The upstream project and attribution are preserved; the restricted-agent behavior described below is specific to this fork.

## Restricted / Agent Setup

This fork supports defense in depth for an MCP client that should manage only one Proxmox resource pool:

1. **Proxmox ACLs are authoritative.** Run the MCP with a dedicated Proxmox user and API token. Do not use `root@pam` or an Administrator token.
2. **The MCP adds a second boundary.** When `PROXMOX_ALLOWED_POOL` is set, `proxmox-mcp` verifies pool membership before VM/LXC mutations and rejects unsafe destination operations locally.
3. **The layers are independent.** The MCP rejects calls outside its configured pool; Proxmox independently evaluates the API token’s ACLs. Security must not depend on Hermes behaving correctly.
4. **The intended result is fail-closed.** An incorrect or unexpectedly broad MCP call still cannot mutate resources outside the pool if the Proxmox ACLs are also correctly scoped.

Do not grant broad VM administration or allocation privileges at `/` or `/vms`. That would weaken or defeat the pool-based ACL boundary. Keep protected infrastructure, including the VM running Hermes, outside the managed pool.

### Example architecture names

The examples below use:

- Proxmox user: `hermes@pve`
- Privilege-separated API token: `hermes@pve!agent`
- Managed pool: `HermesManaged`
- Custom VM role: `HermesVMAdmin`
- Trusted clone-source role: `HermesCloneSource`
- Network-use role: `HermesNetworkUse`
- Trusted golden template: VM `901`, named `debian-13-cloud-qemu`
- VM storage: `local-lvm`
- Network: `vmbr0` in SDN zone `localnetwork`
- MCP setting: `PROXMOX_ALLOWED_POOL=HermesManaged`
- MCP setting: `PROXMOX_ALLOWED_CLONE_SOURCE=901`

These are examples only and can be replaced consistently with names appropriate to your installation.

### Create the Proxmox identity and ACLs

Use a dedicated `pve`-realm user and keep API-token privilege separation enabled. A privilege-separated token’s effective permissions are constrained by the permissions assigned to both the underlying user and the token, so assign every required ACL to both identities. Do not disable privilege separation as a shortcut.

The following commands use the documented `pveum` syntax. Run them as a Proxmox administrator, substitute your own names if needed, and store the token secret securely when it is printed; never commit it. The names and paths below are the validated example deployment, not hard-coded requirements.

```bash
# Dedicated user and privilege-separated token
pveum user add hermes@pve --comment "Hermes Agent"
# The token secret is displayed once; save it in a secret manager.
pveum user token add hermes@pve agent --privsep 1

# Resource pool for agent-managed guests
pveum pool add HermesManaged --comment "VMs managed by Hermes Agent"

# Managed-guest role. VM.Clone permits cloning sources that are themselves
# members of HermesManaged; it does not grant access to VM 901 outside it.
pveum role add HermesVMAdmin --privs "Pool.Audit,VM.Audit,VM.Allocate,VM.Backup,VM.Clone,VM.Config.CDROM,VM.Config.CPU,VM.Config.Disk,VM.Config.Memory,VM.Config.Network,VM.Config.Options,VM.Migrate,VM.PowerMgmt,VM.Snapshot,VM.Snapshot.Rollback"

# Narrow source-template role: audit and clone only.
pveum role add HermesCloneSource --privs "VM.Audit,VM.Clone"

# Narrow network-use role for the validated Proxmox VE 9 SDN layout.
pveum role add HermesNetworkUse --privs "SDN.Use"

# Global read/discovery access: assign to BOTH the user and the token.
pveum acl modify / --roles PVEAuditor --users hermes@pve --propagate 1
pveum acl modify / --roles PVEAuditor --tokens hermes@pve!agent --propagate 1

# Managed-guest administration: assign to BOTH at the pool only.
pveum acl modify /pool/HermesManaged --roles HermesVMAdmin --users hermes@pve --propagate 1
pveum acl modify /pool/HermesManaged --roles HermesVMAdmin --tokens hermes@pve!agent --propagate 1

# Protected golden template: source-side audit and clone only, no propagation.
pveum acl modify /vms/901 --roles HermesCloneSource --users hermes@pve --propagate 0
pveum acl modify /vms/901 --roles HermesCloneSource --tokens hermes@pve!agent --propagate 0

# Network use only for the localnetwork SDN zone and its child vmbr0.
pveum acl modify /sdn/zones/localnetwork --roles HermesNetworkUse --users hermes@pve --propagate 1
pveum acl modify /sdn/zones/localnetwork --roles HermesNetworkUse --tokens hermes@pve!agent --propagate 1

# Destination storage allocation for managed VM disks.
pveum acl modify /storage/local-lvm --roles PVEDatastoreUser --users hermes@pve --propagate 1
pveum acl modify /storage/local-lvm --roles PVEDatastoreUser --tokens hermes@pve!agent --propagate 1
```

`HermesVMAdmin` is assigned only to `/pool/HermesManaged`. It retains the repository’s required managed-VM privileges and adds `VM.Clone` for managed-pool sources plus `Pool.Audit` so the MCP can resolve and inspect actual pool membership. Do not grant it `Pool.Allocate`, `Permissions.Modify`, `Sys.Modify`, `Administrator`, or another broad administration privilege.

`HermesCloneSource` is deliberately separate: VM 901 (`debian-13-cloud-qemu` in this example) remains outside `HermesManaged`, may be inspected and cloned from, but must not receive `VM.Config.*`, `VM.PowerMgmt`, `VM.Snapshot`, `VM.Allocate`, or other mutation privileges. The MCP setting `PROXMOX_ALLOWED_CLONE_SOURCE=901` is an additional defense-in-depth boundary, not a replacement for this ACL isolation.

`HermesNetworkUse` contains only `SDN.Use`. In the validated Proxmox VE 9 layout, the initial clone failed with `Permission check failed (/sdn/zones/localnetwork/vmbr0, SDN.Use)`. Granting it at `/sdn/zones/localnetwork` with propagation enabled permits use of child network `vmbr0` without granting SDN administration. If an installation uses a different network or SDN layout, substitute its narrow relevant ACL path; do not grant `SDN.Use` globally when a narrower zone path is available.

`PVEDatastoreUser` remains the built-in role on `/storage/local-lvm` and supplies the tested destination-storage allocation capability. Installations using different storage must use the corresponding storage path rather than copying `local-lvm` blindly.

More-specific ACLs may need their own audit privilege where required. That is why `HermesCloneSource` explicitly contains `VM.Audit` even though `PVEAuditor` is also assigned at `/`.

Do **not** assign `HermesVMAdmin` at `/`, `/vms`, or another broad path. Do not grant `Pool.Allocate`, `Permissions.Modify`, `Sys.Modify`, or `Administrator` to the agent identities.

### Validated example ACL layout

The resulting ACL entries for the example deployment are:

| Path | Role | Identity | Propagate |
|---|---|---|---|
| `/` | `PVEAuditor` | `hermes@pve` | yes |
| `/` | `PVEAuditor` | `hermes@pve!agent` | yes |
| `/pool/HermesManaged` | `HermesVMAdmin` | `hermes@pve` | yes |
| `/pool/HermesManaged` | `HermesVMAdmin` | `hermes@pve!agent` | yes |
| `/storage/local-lvm` | `PVEDatastoreUser` | `hermes@pve` | yes |
| `/storage/local-lvm` | `PVEDatastoreUser` | `hermes@pve!agent` | yes |
| `/vms/901` | `HermesCloneSource` | `hermes@pve` | no |
| `/vms/901` | `HermesCloneSource` | `hermes@pve!agent` | no |
| `/sdn/zones/localnetwork` | `HermesNetworkUse` | `hermes@pve` | yes |
| `/sdn/zones/localnetwork` | `HermesNetworkUse` | `hermes@pve!agent` | yes |

The same entries can be created in the Proxmox web UI under **Datacenter → Permissions → ACL**, selecting the corresponding path, role, identity, and propagation setting. Keep the user and token entries separate: privilege separation does not make the token inherit the backing user’s ACL entry.

### MCP configuration

```dotenv
PROXMOX_API_URL=https://PROXMOX_HOST:8006/api2/json
PROXMOX_TOKEN_ID=hermes@pve!agent
PROXMOX_TOKEN_SECRET=REPLACE_WITH_SECRET_FROM_TOKEN_CREATION
PROXMOX_INSECURE=true
PROXMOX_ALLOWED_POOL=HermesManaged
# Optional under pool restriction: example trusted template VMID.
PROXMOX_ALLOWED_CLONE_SOURCE=901
# Keep disabled unless destructive tools are deliberately required:
PROXMOX_ALLOW_DESTRUCTIVE=false
```

`PROXMOX_INSECURE=true` is convenient for a local Proxmox installation using its self-signed certificate. Where practical, trust the Proxmox CA or use a properly validated certificate instead. Never store a real token secret in Git or commit `.env`.

### `PROXMOX_ALLOWED_POOL` and clone-source semantics

The current implementation reads these variables once at server startup. When `PROXMOX_ALLOWED_POOL` is unset or empty, the existing unrestricted MCP behavior is preserved, but setting `PROXMOX_ALLOWED_CLONE_SOURCE` without a pool is rejected as inconsistent configuration. When the pool is set:

- Existing VM/LXC mutations first resolve the configured pool through Proxmox and require the target resource to be an actual `qemu` or `lxc` member of that pool. A membership lookup failure rejects the mutation locally.
- `create_vm` requires an explicit `pool` argument. The value must exactly equal `PROXMOX_ALLOWED_POOL`; the MCP does not silently inject the pool.
- `list_pools` explicitly resolves the configured pool instead of relying on unfiltered `GET /pools`, then returns that verified pool. `get_pool` remains available for explicit inspection.
- `create_pool`, `update_pool`, and `delete_pool` are rejected locally while the restriction is enabled, so the agent cannot modify its own security boundary.
- `clone_vm` requires an explicit `pool` argument. The value must exactly equal `PROXMOX_ALLOWED_POOL`, and it is sent atomically as the Proxmox clone operation’s destination `pool` field; the MCP never creates outside the pool and moves afterward.
- A source VM inside `HermesManaged` may be cloned with `VM.Clone` through `HermesVMAdmin`. The configured `PROXMOX_ALLOWED_CLONE_SOURCE` is the only exception that permits a source outside the pool, such as example VM 901.
- VM 901 remains subject to the normal pool boundary for start, stop, configure, resize, migrate, disk move, snapshot, delete, firewall, and other mutations. The external-source exception is only for the clone-source path.
- `clone_container`, `restore_vm`, `restore_container`, and `create_container` remain rejected while restricted because their current requests cannot safely guarantee destination membership in the configured pool. VM/container backup remains subject to source membership verification.
- Destructive tools are controlled separately by `PROXMOX_ALLOW_DESTRUCTIVE` and are disabled by default. If enabled, destructive VM/container operations still pass through the pool boundary.
- Read-only operations remain governed by the Proxmox API token’s ACLs; the MCP-side restriction is primarily a mutation boundary.

The two security layers are independent:

- **Proxmox ACL layer:** VM 901 has audit and clone only; `HermesManaged` has managed-VM administration; `local-lvm` has the required storage allocation role; `localnetwork` has network use only. Other VMs, nodes, storage, and administrative resources remain outside these grants.
- **MCP layer:** `PROXMOX_ALLOWED_POOL=HermesManaged`; `PROXMOX_ALLOWED_CLONE_SOURCE=901`; clone destination pool is mandatory and must match exactly; 901 is exceptional only as a clone source; normal mutations of 901 remain rejected locally; destructive tools remain independently disabled unless explicitly enabled.

Neither layer replaces the other. The ACLs protect the live Proxmox API boundary, while the MCP checks provide a second local fail-closed boundary.

### Validated example deployment

Against the documented example Proxmox VE 9 installation, the privilege-separated token was tested to:

- Read protected template VM 901: allowed.
- Mutate protected template VM 901: denied by Proxmox with HTTP 403.
- Full-clone VM 901: allowed.
- Send the clone destination atomically as `pool=HermesManaged`: confirmed.
- Complete the clone task with `exitstatus OK`: confirmed.
- Produce a VM directly as a member of `HermesManaged`: confirmed.
- Use `vmbr0` through the narrow propagated `SDN.Use` ACL: confirmed.
- Allocate the destination disk on `local-lvm`: confirmed.
- Inspect `HermesManaged` with `Pool.Audit`: confirmed.

These are results from the documented example deployment. Repeat equivalent positive and negative tests after adapting the identities, pool, template, storage, and SDN paths to another environment.

### Verify the boundary before trusting the agent

With the MCP configured, verify at minimum that:

1. A managed-pool VM can be mutated and cloned to an explicitly matching `HermesManaged` destination.
2. A clone with an omitted or different destination pool fails locally.
3. VM 901 can be read and cloned, but its ordinary mutation attempts fail both locally and at Proxmox.
4. An arbitrary VM outside `HermesManaged` cannot be mutated or used as a clone source.
5. A clone using `vmbr0` completes, reports `exitstatus OK`, allocates on `local-lvm`, and appears directly in `HermesManaged`.

Repeat these tests for your own ACL paths; the example values are not hard-coded requirements.

## Tools

### Cluster & Nodes

| Tool | Description | Parameters |
|---|---|---|
| `list_nodes` | List all nodes in the cluster | — |
| `get_node_status` | Detailed status of a node | `node` |
| `list_cluster_resources` | All resources across the cluster | `type` (optional: `vm`, `storage`, `node`, `sdn`) |
| `get_cluster_status` | Cluster status and quorum information | — |
| `list_ha_groups` | List all HA node-affinity rules (the modern replacement for legacy HA groups) | — |
| `list_ha_resources` | List all HA-managed resources (VMs and containers) | — |
| `get_ha_status` | Current HA manager status, including quorum and per-node/per-resource state | — |
| `list_cluster_config_nodes` | Corosync nodelist (node names, IDs, ring addresses) | — |
| `list_node_storage` | Storage pools available on a node | `node` |
| `list_node_tasks` | Recent tasks on a node | `node`, `limit` (optional) |
| `get_node_disks` | Physical disks detected on a node | `node` |
| `get_disk_smart` | SMART health data for a single disk (attributes, health, error counters) | `node`, `disk` (device path, e.g. `/dev/sda`) |
| `list_zfs_pools` | List all ZFS pools on a node with health status | `node` |
| `get_zfs_pool` | Detailed ZFS pool status including per-device health (like `zpool status -v`) | `node`, `name` (pool name) |
| `get_node_journal` | Raw systemd journal entries for a node (e.g. auditing SSH/PAM auth activity) | `node`, `since` (optional, Unix timestamp), `until` (optional, Unix timestamp), `last_entries` (optional) |

### QEMU VMs

| Tool | Description | Parameters |
|---|---|---|
| `list_vms` | QEMU VMs on a node | `node` |
| `get_vm_status` | VM status and current config | `node`, `vmid` |
| `get_vm_config` | Full VM configuration | `node`, `vmid` |
| `start_vm` | Start a VM (returns task UPID) | `node`, `vmid` |
| `stop_vm` | Hard stop a VM (returns task UPID) | `node`, `vmid` |
| `shutdown_vm` | Graceful ACPI shutdown (returns task UPID) | `node`, `vmid` |
| `reboot_vm` | Reboot a VM (returns task UPID) | `node`, `vmid` |
| `suspend_vm` | Suspend a VM (returns task UPID) | `node`, `vmid` |
| `resume_vm` | Resume a suspended VM (returns task UPID) | `node`, `vmid` |
| `list_vm_snapshots` | List all snapshots for a VM | `node`, `vmid` |
| `create_vm_snapshot` | Create a VM snapshot (returns task UPID) | `node`, `vmid`, `snapname`, `description` (optional) |
| `rollback_vm_snapshot` | Roll back a VM to a snapshot (returns task UPID) | `node`, `vmid`, `snapname` |
| `delete_vm_snapshot` | Delete a VM snapshot (returns task UPID) | `node`, `vmid`, `snapname` |
| `create_vm` | Create a new QEMU VM (returns task UPID) | `node`, `vmid`, `name` (optional), `pool` (optional), `memory` (optional), `cores` (optional), `iso` (optional), `disk` (optional), `net0` (optional), `start` (optional) |
| `clone_vm` | Clone a VM to a new ID (returns task UPID) | `node`, `vmid`, `newid`, `name` (optional), `target_node` (optional) |
| `set_vm_config` | Update VM config (sync, no task) | `node`, `vmid`, `name` (optional), `memory` (optional), `cores` (optional), `onboot` (optional), `description` (optional) |
| `resize_vm_disk` | Resize a VM disk (returns task UPID) | `node`, `vmid`, `disk` (e.g. `scsi0`), `size` (e.g. `+10G` or `50G`) |
| `migrate_vm` | Migrate a VM to another node (returns task UPID) | `node`, `vmid`, `target`, `online` (optional, live migrate) |
| `restore_vm` | Restore a VM from a vzdump backup archive (returns task UPID) | `node`, `vmid`, `archive` (volid), `storage` (optional), `start` (optional) |
| `move_vm_disk` | Move a VM disk to a different storage pool (returns task UPID) | `node`, `vmid`, `disk` (e.g. `scsi0`), `storage` (target pool), `delete_source` (optional) |

### LXC Containers

| Tool | Description | Parameters |
|---|---|---|
| `list_containers` | LXC containers on a node | `node` |
| `get_container_status` | Container status | `node`, `vmid` |
| `get_container_config` | Full container configuration | `node`, `vmid` |
| `start_container` | Start a container (returns task UPID) | `node`, `vmid` |
| `stop_container` | Stop a container (returns task UPID) | `node`, `vmid` |
| `shutdown_container` | Graceful ACPI shutdown (returns task UPID) | `node`, `vmid` |
| `reboot_container` | Reboot a container (returns task UPID) | `node`, `vmid` |
| `list_container_snapshots` | List all snapshots for a container | `node`, `vmid` |
| `create_container_snapshot` | Create a container snapshot (returns task UPID) | `node`, `vmid`, `snapname`, `description` (optional) |
| `rollback_container_snapshot` | Roll back a container to a snapshot (returns task UPID) | `node`, `vmid`, `snapname` |
| `delete_container_snapshot` | Delete a container snapshot (returns task UPID) | `node`, `vmid`, `snapname` |
| `create_container` | Create a new LXC container (returns task UPID) | `node`, `vmid`, `ostemplate`, `hostname` (optional), `memory` (optional), `rootfs` (optional), `password` (optional), `net0` (optional), `start` (optional) |
| `clone_container` | Clone a container to a new ID (returns task UPID) | `node`, `vmid`, `newid`, `hostname` (optional), `target_node` (optional) |
| `set_container_config` | Update container config (sync, no task) | `node`, `vmid`, `hostname` (optional), `memory` (optional), `swap` (optional), `onboot` (optional), `description` (optional) |
| `resize_container_disk` | Resize a container disk (returns task UPID) | `node`, `vmid`, `disk` (e.g. `rootfs`), `size` (e.g. `+5G` or `10G`) |
| `migrate_container` | Migrate a container to another node (returns task UPID) | `node`, `vmid`, `target`, `restart` (optional, stop+migrate+start) |
| `restore_container` | Restore a container from a vzdump backup archive (returns task UPID) | `node`, `vmid`, `archive` (volid), `storage` (optional), `hostname` (optional), `start` (optional) |

### Backups

| Tool | Description | Parameters |
|---|---|---|
| `create_backup` | Create a vzdump backup of a VM or container (returns task UPID) | `node`, `vmid`, `storage` (optional), `mode` (optional: `snapshot`\|`suspend`\|`stop`, default `snapshot`), `compress` (optional: `zstd`\|`gzip`\|`lzo`\|`0`, default `zstd`) |
| `list_backups` | List all backup volumes in a storage pool | `node`, `storage` |

### Tasks

| Tool | Description | Parameters |
|---|---|---|
| `get_task_status` | Poll the status of an async task | `node`, `upid` |

### Network

| Tool | Description | Parameters |
|---|---|---|
| `list_node_network` | List network interfaces on a node | `node`, `type` (optional: `bridge`, `bond`, `eth`, `alias`, `vlan`, `OVSBridge`, `OVSBond`, `OVSPort`, `OVSIntPort`, `any_bridge`) |
| `get_node_network_interface` | Get configuration of a specific network interface | `node`, `iface` (e.g. `vmbr0`) |
| `create_node_network_interface` | Create a new network interface on a node (staged until `apply_node_network_changes`) | `node`, `iface`, `type` (`bridge`\|`bond`\|`eth`\|`alias`\|`vlan`\|`OVSBridge`\|`OVSBond`\|`OVSPort`\|`OVSIntPort`), `address` (optional, dotted-decimal or CIDR), `netmask` (optional), `gateway` (optional), `address6` (optional, CIDR), `gateway6` (optional), `mtu` (optional), `autostart` (optional, `1`=boot, `0`=manual), `bridge_ports` (optional), `bridge_stp` (optional), `bridge_fd` (optional), `bond_mode` (optional), `slaves` (optional), `comments` (optional) |
| `update_node_network_interface` | Update an existing network interface on a node (staged until `apply_node_network_changes`) | `node`, `iface`, `type` (required: same values as `create_node_network_interface`), plus any optional fields as in `create_node_network_interface` |
| `apply_node_network_changes` | Apply all staged network configuration changes on a node, reloading the network stack | `node` |

### Firewall

| Tool | Description | Parameters |
|---|---|---|
| `list_cluster_firewall_rules` | List all firewall rules at the datacenter level | — |
| `get_cluster_firewall_options` | Get datacenter firewall policy options (default in/out policies, logging) | — |
| `list_vm_firewall_rules` | List all firewall rules for a QEMU VM | `node`, `vmid` |
| `get_vm_firewall_options` | Get firewall policy options for a QEMU VM | `node`, `vmid` |
| `list_container_firewall_rules` | List all firewall rules for an LXC container | `node`, `vmid` |
| `get_container_firewall_options` | Get firewall policy options for an LXC container | `node`, `vmid` |
| `add_vm_firewall_rule` | Add a firewall rule to a QEMU VM | `node`, `vmid`, `type` (`in`\|`out`), `action` (`ACCEPT`\|`DROP`\|`REJECT`), `proto` (optional), `dport` (optional), `sport` (optional), `source` (optional), `dest` (optional), `iface` (optional), `comment` (optional), `enable` (optional) |
| `delete_vm_firewall_rule` | Delete a firewall rule from a QEMU VM by position | `node`, `vmid`, `pos` (zero-based) |
| `add_container_firewall_rule` | Add a firewall rule to an LXC container | `node`, `vmid`, `type`, `action`, `proto` (optional), `dport` (optional), `sport` (optional), `source` (optional), `dest` (optional), `iface` (optional), `comment` (optional), `enable` (optional) |
| `delete_container_firewall_rule` | Delete a firewall rule from an LXC container by position | `node`, `vmid`, `pos` (zero-based) |

### Pool Management

| Tool | Description | Parameters |
|---|---|---|
| `list_pools` | List all resource pools in the cluster | — |
| `get_pool` | Get full details of a pool including its member VMs, containers, and storage | `poolid` |
| `create_pool` | Create a new resource pool | `poolid`, `comment` (optional) |
| `update_pool` | Update a pool: change comment or add/remove member VMs and storage | `poolid`, `comment` (optional), `vms` (optional, comma-separated VM/CT IDs), `storage` (optional, comma-separated storage names), `delete` (optional, set `true` to remove listed members instead of adding) |

### Storage Content

| Tool | Description | Parameters |
|---|---|---|
| `list_storage_content` | List volumes in a storage pool | `node`, `storage`, `content` (optional: `iso`, `vztmpl`, `backup`, `images`) |
| `get_storage_content_info` | Detailed info about a specific volume | `node`, `storage`, `volume` (full volid, e.g. `local:iso/debian.iso`) |

### Storage Definitions

| Tool | Description | Parameters |
|---|---|---|
| `list_storages` | List all cluster-wide storage definitions | `type` (optional filter: `nfs`, `pbs`, `dir`, `cifs`, `zfspool`, etc.) |
| `get_storage` | Get full configuration of a storage definition | `storage` (name) |
| `add_storage` | Add a new storage target to the cluster | `storage` (name), `type` (required: `nfs`\|`pbs`\|`dir`\|`cifs`\|`zfspool`\|...), `server` (optional), `export` (optional, NFS path), `path` (optional, dir path), `datastore` (optional, PBS datastore), `username` (optional), `password` (optional), `fingerprint` (optional, PBS TLS fingerprint), `content` (optional, e.g. `backup,images`), `nodes` (optional, comma-sep), `shared` (optional bool) |
| `update_storage` | Update an existing storage definition | `storage` (name), plus any of: `server`, `export`, `path`, `datastore`, `username`, `password`, `fingerprint`, `content`, `nodes`, `shared` |

### Access Control

| Tool | Description | Parameters |
|---|---|---|
| `list_users` | List all users across all realms (pve, pam, ldap, etc) — audit for unrecognized or orphaned accounts | — |
| `list_user_tokens` | List API tokens issued to a user (metadata only, secrets are never returned) | `userid` (e.g. `root@pam`) |

### Destructive (opt-in)

These tools are **not registered by default**. Set `PROXMOX_ALLOW_DESTRUCTIVE=true` to enable them.

| Tool | Description | Parameters |
|---|---|---|
| `delete_vm` | Permanently delete a stopped QEMU VM (returns task UPID) | `node`, `vmid`, `confirmed` (must be `true`), `purge` (optional) |
| `delete_container` | Permanently delete a stopped LXC container (returns task UPID) | `node`, `vmid`, `confirmed` (must be `true`), `purge` (optional) |
| `delete_storage_content` | Permanently delete a volume from a storage pool (returns task UPID) | `node`, `storage`, `volume` (full volid), `confirmed` (must be `true`) |
| `reboot_node` | Reboot an entire Proxmox node | `node`, `confirmed` (must be `true`) |
| `shutdown_node` | Shut down an entire Proxmox node | `node`, `confirmed` (must be `true`) |
| `delete_pool` | Permanently delete an empty resource pool | `poolid`, `confirmed` (must be `true`) |
| `remove_storage` | Remove a storage definition from the cluster (does not affect underlying data) | `storage` (name), `confirmed` (must be `true`) |
| `delete_node_network_interface` | Remove a network interface from a node (staged until `apply_node_network_changes`) | `node`, `iface`, `confirmed` (must be `true`) |

Lifecycle and snapshot operations are non-blocking — they return the UPID of the async task immediately. Use `get_task_status` to poll for completion.

## Installation

### Download a pre-built binary

Download the latest release for your platform from the [Releases](https://github.com/gordcurrie/proxmox-mcp/releases) page.

| Platform | Binary |
|---|---|
| Linux (amd64) | `proxmox-mcp_linux_amd64` |
| Linux (arm64) | `proxmox-mcp_linux_arm64` |
| macOS (amd64) | `proxmox-mcp_darwin_amd64` |
| macOS (arm64) | `proxmox-mcp_darwin_arm64` |
| Windows (amd64) | `proxmox-mcp_windows_amd64.exe` |

Make it executable and place it on your `PATH` (substitute the filename for your platform):

```bash
chmod +x <binary-name>
mv <binary-name> /usr/local/bin/proxmox-mcp
```

> Windows users: rename the `.exe` and add it to a directory on your `%PATH%`.

### Build from source

Requires Go 1.26+. You will also need a Proxmox VE API token — create one in **Datacenter → Permissions → API Tokens**.

```bash
git clone https://github.com/gordcurrie/proxmox-mcp
cd proxmox-mcp
cp .env.example .env   # copy the example env file
$EDITOR .env           # set PROXMOX_* values (see table below)
make build             # binary lands in bin/proxmox-mcp
```

## Configuration

All configuration is via environment variables:

| Variable | Required | Description |
|---|---|---|
| `PROXMOX_API_URL` | yes | e.g. `https://pve:8006/api2/json` |
| `PROXMOX_TOKEN_ID` | yes | e.g. `user@realm!tokenid` (use a dedicated non-root identity) |
| `PROXMOX_TOKEN_SECRET` | yes | Token UUID secret |
| `PROXMOX_INSECURE` | no | `true` to skip TLS verification (self-signed certs) |
| `PROXMOX_ALLOW_DESTRUCTIVE` | no | `true` to register `delete_vm`, `delete_container`, `delete_storage_content`, `reboot_node`, `shutdown_node`, and `delete_pool` tools (default: disabled) |
| `PROXMOX_ALLOWED_POOL` | no | Restrict resource-pool management to the named pool. When set, VM/container mutations verify membership, `create_vm` requires an explicit matching `pool`, pool mutations are rejected, and `clone_vm` requires an explicit matching destination pool. The pool is still verified through Proxmox; unset preserves normal behavior. |
| `PROXMOX_ALLOWED_CLONE_SOURCE` | no | One VMID permitted as an external `clone_vm` source while pool restriction is active (example: `901`). Requires `PROXMOX_ALLOWED_POOL`; malformed or inconsistent configuration fails at startup. It does not grant ordinary mutation rights. |

Source your `.env` file before running:

```bash
set -a && source .env && set +a
```

## Running

### stdio (default — for local MCP clients)

```bash
./bin/proxmox-mcp
```

### HTTP (streamable — for remote/shared deployments)

```bash
./bin/proxmox-mcp --transport http --addr localhost:8080
```

## VS Code Copilot configuration

Create `.vscode/mcp.json` in your workspace (already gitignored):

```json
{
  "servers": {
    "proxmox-mcp": {
      "type": "stdio",
      "command": "/path/to/proxmox-mcp/bin/proxmox-mcp",
      "env": {
        "PROXMOX_API_URL": "https://your-proxmox-host:8006/api2/json",
        "PROXMOX_TOKEN_ID": "user@realm!tokenid",
        "PROXMOX_TOKEN_SECRET": "your-token-secret",
        "PROXMOX_ALLOWED_POOL": "HermesManaged"
      }
    }
  }
}
```

Then open the Copilot chat panel, switch to **Agent** mode, and the `proxmox-mcp` server will appear in the available tools.

## Claude Desktop configuration

Add the server to `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) or `%APPDATA%\Claude\claude_desktop_config.json` (Windows):

```json
{
  "mcpServers": {
    "proxmox-mcp": {
      "command": "/path/to/proxmox-mcp/bin/proxmox-mcp",
      "env": {
        "PROXMOX_API_URL": "https://your-proxmox-host:8006/api2/json",
        "PROXMOX_TOKEN_ID": "user@realm!tokenid",
        "PROXMOX_TOKEN_SECRET": "your-token-secret",
        "PROXMOX_ALLOWED_POOL": "HermesManaged"
      }
    }
  }
}
```

Restart Claude Desktop after saving the config — the Proxmox tools will appear in the tool selector.

## OpenCode configuration

Add the server to `opencode.json` in your project root (or `~/.config/opencode/opencode.json` for global config):

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "proxmox-mcp": {
      "type": "local",
      "command": ["/path/to/proxmox-mcp/bin/proxmox-mcp"],
      "enabled": true,
      "environment": {
        "PROXMOX_API_URL": "https://your-proxmox-host:8006/api2/json",
        "PROXMOX_TOKEN_ID": "user@realm!tokenid",
        "PROXMOX_TOKEN_SECRET": "your-token-secret",
        "PROXMOX_ALLOWED_POOL": "HermesManaged"
      }
    }
  }
}
```

## Development

```bash
make install-tools   # install golangci-lint, gosec, govulncheck, gofumpt
make check           # full quality gate: fix, fmt, vet, lint, sec, vulncheck, test, build
make test            # tests only (with race detector)
make build           # build only → bin/proxmox-mcp
make clean           # remove bin/proxmox-mcp
```
