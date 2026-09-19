---
name: proxmox-vm-provisioning
description: Provision Debian VMs through a constrained Proxmox MCP.
version: 0.1.0
author: Deployment maintainer, Hermes Agent
license: MIT
platforms: [linux, macos, windows]
metadata:
  hermes:
    tags: [Proxmox, VMs, Debian, cloud-init, MCP, provisioning]
    related_skills: []
---

# Proxmox VM Provisioning

Use this skill when Hermes must create and provision a Debian VM through the
Proxmox MCP. It covers the complete clone, configure, cloud-init, boot,
guest-agent, SSH, application-installation, and verification workflow. It does
not authorize bypassing MCP or Proxmox ACL boundaries.

## Reference deployment configuration

The following values come from one validated deployment and are examples, not
universal requirements:

- managed pool: `HermesManaged`
- trusted Debian template VMID: `901`
- protected Hermes VMID: `100`
- guest user: `hermes-admin`
- private SSH identity: `~/.ssh/hermes_managed_ed25519`
- public SSH identity: `~/.ssh/hermes_managed_ed25519.pub`

Before using this skill elsewhere, replace these values with the installation's
reviewed pool, clone source, protected agent VM, guest user, and dedicated SSH
identity. Preserve the same separation: the agent is outside the managed pool,
the approved source is clone-only, and every destination is explicitly managed.

## Security rules

- Use Proxmox operations only through the Proxmox MCP tools.
- Never use Proxmox REST calls, `curl`, `pvesh`, SSH to the Proxmox host, or a
  more privileged administrative interface to bypass an MCP or ACL denial.
- Enforce both layers: the MCP allowed-pool/clone-source boundary and narrow
  Proxmox ACLs. Report a denial instead of working around it.
- Never mutate the trusted template, the Hermes VM, or unrelated infrastructure.
- Use a dedicated provisioning key only. Never use `~/.ssh/id_ed25519` or a
  personal/default identity, and never read, print, log, or commit private-key
  contents.
- Destructive tools remain disabled by default. Do not delete or repurpose an
  existing VM without explicit authorization and an allowed MCP capability.
- Do not overwrite an existing VMID; select an unused destination from MCP
  read-only information.

## MCP tools

Use the current tool names and their required fields:

- `list_cluster_resources` with `type: "vm"` for read-only VMID discovery.
- `get_pool` with `poolid` to verify the managed pool and membership.
- `get_vm_status` and `get_vm_config` with `node` and `vmid` for verification.
- `clone_vm` with `node`, source `vmid`, `newid`, and explicit `full: true`;
  pass the exact managed `pool` and optionally `name` or `target_node`.
- `get_task_status` with `node` and `upid` for every asynchronous operation.
- `set_vm_config` with `node`, `vmid`, and requested `cores`/`memory`.
- `resize_vm_disk` with `node`, `vmid`, `disk`, and a non-shrinking `size`.
- `set_vm_cloudinit` with `node`, `vmid`, `ciuser`, raw `sshkeys`, and
  `ipconfig0`.
- `start_vm` with `node` and `vmid`.
- `get_vm_guest_network_interfaces` with `node` and `vmid` for guest-agent
  network discovery.

Do not guess node names or schemas; inspect the live MCP tool schema and use
node names returned by read-only discovery.

## Provisioning procedure

1. Determine the target node and an unused VMID using MCP read-only discovery.
   Verify the configured managed pool exists. Completion criterion: the VMID
   does not appear in cluster resources and the destination pool is known.
2. Read the protected template status/configuration and verify its identity.
   Verify the requested source is the configured trusted source and do not call
   any mutation against it. Completion criterion: source identity is confirmed.
3. Call `clone_vm` with `full: true` and the exact managed destination `pool`.
   If it returns a UPID, poll `get_task_status` until `status` is terminal and
   require `exitstatus: "OK"`. Completion criterion: the new VM exists in the
   managed pool and the source remains unchanged.
4. Apply the requested CPU and memory with `set_vm_config`. Read back the
   configuration when appropriate and confirm the requested values. Completion
   criterion: the target config contains the requested cores and memory.
5. Inspect the primary disk and use `resize_vm_disk` only when the requested
   size is larger. Poll its UPID to successful completion and verify the disk;
   never shrink it. Completion criterion: the target disk is at least requested
   size.
6. Read the public key locally with `read_file` or `terminal` only when needed.
   Never read the private key. Call `set_vm_cloudinit` with:
   `ciuser=hermes-admin` (or the configured guest user),
   `ipconfig0=ip=dhcp`, and the normal raw contents of the configured `.pub`
   file. Do not URL-encode it. The MCP performs Proxmox `sshkeys` encoding.
   Read back the VM config without printing the complete key. Completion
   criterion: cloud-init user, DHCP setting, and SSH-key presence are verified.
7. Call `start_vm`, poll any returned UPID to `exitstatus: "OK"`, and verify
   `get_vm_status` reports `status: "running"`. Completion criterion: VM is
   running.
8. Query `get_vm_guest_network_interfaces`. The guest agent may need time to
   start, so retry for a bounded reasonable interval. Select a primary
   non-loopback IPv4 address and record the interface, MAC, address, and prefix.
   Note non-link-local IPv6 when present. Do not use ARP, scans, router data,
   DHCP tables, Home Assistant, or another fallback. Completion criterion: the
   address came from the MCP guest-agent tool.
9. Only after MCP supplies the address, use `terminal` for local SSH as the
   configured guest user with the dedicated private key and `IdentitiesOnly=yes`.
   Do not fall back to passwords. Capture authentication evidence showing that
   the dedicated key was offered and accepted. Verify `sudo -n true` before
   installing anything. Completion criterion: authenticated SSH and
   non-interactive sudo succeed.
10. Install only the requested application in the guest using normal Debian
    administration. Do not make unrelated guest changes. Completion criterion:
    requested service/configuration is present.
11. Verify final state: VM running, cloud-init completed, requested CPU/RAM/disk
    visible, `qemu-guest-agent` active, application service active and enabled
    when appropriate, and the requested endpoint or health check working.
    Report VM identity, discovered address, application state, and caveats.

## SSH verification baseline

For a new Debian guest, use read-only checks such as:

- `hostname`, `whoami`, `nproc`, `free -h`
- `lsblk`, `df -h /`
- `cloud-init status`
- `sudo -n true`
- `systemctl is-active qemu-guest-agent`

Do not treat an installation command returning zero as sufficient verification.

## ACL and boundary model

The MCP configuration must name an allowed managed pool and, where applicable,
an allowed external clone source. The Proxmox API token must independently have
only the required audit/clone permissions on the trusted source, managed-VM
permissions on the managed pool, narrow network use, and destination storage
allocation. Keep the Hermes VM and unrelated infrastructure outside the pool.
Do not grant broad administrator, root, ACL-modification, host-networking, or
host-storage privileges.

The reference deployment validated full cloning, direct pool placement,
asynchronous task polling, CPU/RAM configuration, disk expansion, raw-key
cloud-init, DHCP, startup, QEMU guest-agent discovery, dedicated-key SSH,
passwordless sudo, application installation, systemd enable/start, and service
verification on Debian 13. Omit disposable acceptance-test IDs, addresses, and
MACs from reusable documentation.

## MCP upgrade lesson

Replacing `/usr/local/bin/proxmox-mcp` does not replace an already-running
process. A process can continue executing an old inode and
`/proc/<pid>/exe` can show `(deleted)`. After an MCP upgrade, use the normal
Hermes lifecycle mechanism to restart the relevant MCP server, inspect running
processes, ensure none executes a deleted binary, and verify running-build
provenance before acceptance testing. Do not kill or restart processes as part
of ordinary VM provisioning.

## Failure handling

Stop and report the exact error when MCP or Proxmox denies an operation, the
source/pool boundary cannot be verified, a task does not finish successfully,
cloud-init cannot be read back, guest-agent discovery fails after bounded
retries, or dedicated-key SSH fails. Never repair these conditions by using a
more privileged interface or by weakening the boundary.
