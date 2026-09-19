package tools

import (
	"context"
	"fmt"

	"github.com/gordcurrie/proxmox-mcp/internal/proxmox"
)

// poolRestrictedClient enforces an optional management boundary before any
// resource-pool member mutation is sent to Proxmox.
type poolRestrictedClient struct {
	proxmoxClient
	allowedPool        string
	allowedCloneSource int
}

func (c *poolRestrictedClient) verifyMember(ctx context.Context, kind string, vmid int) error {
	pool, err := c.GetPool(ctx, c.allowedPool)
	if err != nil {
		return fmt.Errorf("verify %s %d pool membership: %w", kind, vmid, err)
	}
	for _, member := range pool.Members {
		if member.VMID == vmid && ((kind == "VM" && member.Type == "qemu") || (kind == "container" && member.Type == "lxc")) {
			return nil
		}
	}
	return fmt.Errorf("%s %d is not a member of allowed pool %q", kind, vmid, c.allowedPool)
}

func (c *poolRestrictedClient) verifyVM(ctx context.Context, vmid int) error {
	return c.verifyMember(ctx, "VM", vmid)
}

func (c *poolRestrictedClient) verifyContainer(ctx context.Context, vmid int) error {
	return c.verifyMember(ctx, "container", vmid)
}

func (c *poolRestrictedClient) verifyAnyMember(ctx context.Context, vmid int) error {
	pool, err := c.GetPool(ctx, c.allowedPool)
	if err != nil {
		return fmt.Errorf("verify resource %d pool membership: %w", vmid, err)
	}
	for _, member := range pool.Members {
		if member.VMID == vmid && (member.Type == "qemu" || member.Type == "lxc") {
			return nil
		}
	}
	return fmt.Errorf("resource %d is not a member of allowed pool %q", vmid, c.allowedPool)
}

func (c *poolRestrictedClient) ListPools(ctx context.Context) ([]proxmox.Pool, error) {
	pool, err := c.GetPool(ctx, c.allowedPool)
	if err != nil {
		return nil, err
	}
	return []proxmox.Pool{*pool}, nil
}

func (c *poolRestrictedClient) CreatePool(ctx context.Context, req *proxmox.CreatePoolRequest) error {
	return fmt.Errorf("create_pool is unavailable when allowed pool %q is set: creating another pool would exceed the management boundary", c.allowedPool)
}

func (c *poolRestrictedClient) UpdatePool(ctx context.Context, poolid string, req *proxmox.UpdatePoolRequest) error {
	return fmt.Errorf("update_pool is unavailable when allowed pool %q is set: resource pools are immutable through the restricted client", c.allowedPool)
}

func (c *poolRestrictedClient) DeletePool(ctx context.Context, poolid string) error {
	return fmt.Errorf("delete_pool is unavailable when allowed pool %q is set: resource pools are immutable through the restricted client", c.allowedPool)
}

func (c *poolRestrictedClient) CreateVM(ctx context.Context, node string, req *proxmox.CreateVMRequest) (string, error) {
	if req == nil {
		return "", fmt.Errorf("create_vm request is nil")
	}
	if req.Pool == "" {
		return "", fmt.Errorf("pool must be explicitly specified and must equal allowed pool %q", c.allowedPool)
	}
	if req.Pool != c.allowedPool {
		return "", fmt.Errorf("pool %q is not allowed; must equal allowed pool %q", req.Pool, c.allowedPool)
	}
	return c.proxmoxClient.CreateVM(ctx, node, req)
}

func (c *poolRestrictedClient) CloneVM(ctx context.Context, node string, vmid int, req *proxmox.CloneVMRequest) (string, error) {
	if req == nil {
		return "", fmt.Errorf("clone_vm request is nil")
	}
	if req.Pool == "" {
		return "", fmt.Errorf("clone_vm destination pool must be explicitly specified and must equal allowed pool %q", c.allowedPool)
	}
	if req.Pool != c.allowedPool {
		return "", fmt.Errorf("clone_vm destination pool %q is not allowed; must equal allowed pool %q", req.Pool, c.allowedPool)
	}
	if vmid != c.allowedCloneSource {
		if err := c.verifyVM(ctx, vmid); err != nil {
			return "", err
		}
	}
	return c.proxmoxClient.CloneVM(ctx, node, vmid, req)
}

func (c *poolRestrictedClient) CreateContainer(ctx context.Context, node string, req *proxmox.CreateContainerRequest) (string, error) {
	return "", fmt.Errorf("create_container is unavailable when allowed pool %q is set: the Proxmox request cannot explicitly bind the new container to that pool", c.allowedPool)
}

func (c *poolRestrictedClient) CloneContainer(ctx context.Context, node string, vmid int, req *proxmox.CloneContainerRequest) (string, error) {
	if err := c.verifyContainer(ctx, vmid); err != nil {
		return "", err
	}
	return "", fmt.Errorf("clone_container is unavailable when allowed pool %q is set: the Proxmox clone request cannot explicitly bind the destination to that pool", c.allowedPool)
}

func (c *poolRestrictedClient) RestoreVM(ctx context.Context, node string, req *proxmox.RestoreVMRequest) (string, error) {
	return "", fmt.Errorf("restore_vm is unavailable when allowed pool %q is set: the Proxmox restore request cannot explicitly bind the new VM to that pool", c.allowedPool)
}

func (c *poolRestrictedClient) RestoreContainer(ctx context.Context, node string, req *proxmox.RestoreContainerRequest) (string, error) {
	return "", fmt.Errorf("restore_container is unavailable when allowed pool %q is set: the Proxmox restore request cannot explicitly bind the new container to that pool", c.allowedPool)
}

func (c *poolRestrictedClient) CreateBackup(ctx context.Context, node string, req *proxmox.CreateBackupRequest) (string, error) {
	if req == nil {
		return "", fmt.Errorf("create_backup request is nil")
	}
	if err := c.verifyAnyMember(ctx, req.VMID); err != nil {
		return "", err
	}
	return c.proxmoxClient.CreateBackup(ctx, node, req)
}

func (c *poolRestrictedClient) StartVM(ctx context.Context, node string, vmid int) (string, error) {
	if err := c.verifyVM(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.StartVM(ctx, node, vmid)
}

func (c *poolRestrictedClient) StopVM(ctx context.Context, node string, vmid int) (string, error) {
	if err := c.verifyVM(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.StopVM(ctx, node, vmid)
}

func (c *poolRestrictedClient) ShutdownVM(ctx context.Context, node string, vmid int) (string, error) {
	if err := c.verifyVM(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.ShutdownVM(ctx, node, vmid)
}

func (c *poolRestrictedClient) RebootVM(ctx context.Context, node string, vmid int) (string, error) {
	if err := c.verifyVM(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.RebootVM(ctx, node, vmid)
}

func (c *poolRestrictedClient) SuspendVM(ctx context.Context, node string, vmid int) (string, error) {
	if err := c.verifyVM(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.SuspendVM(ctx, node, vmid)
}

func (c *poolRestrictedClient) ResumeVM(ctx context.Context, node string, vmid int) (string, error) {
	if err := c.verifyVM(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.ResumeVM(ctx, node, vmid)
}

func (c *poolRestrictedClient) SetVMConfig(ctx context.Context, node string, vmid int, req *proxmox.SetVMConfigRequest) error {
	if err := c.verifyVM(ctx, vmid); err != nil {
		return err
	}
	return c.proxmoxClient.SetVMConfig(ctx, node, vmid, req)
}

func (c *poolRestrictedClient) ResizeVMDisk(ctx context.Context, node string, vmid int, req *proxmox.ResizeDiskRequest) (string, error) {
	if err := c.verifyVM(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.ResizeVMDisk(ctx, node, vmid, req)
}

func (c *poolRestrictedClient) MigrateVM(ctx context.Context, node string, vmid int, req *proxmox.MigrateVMRequest) (string, error) {
	if err := c.verifyVM(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.MigrateVM(ctx, node, vmid, req)
}

func (c *poolRestrictedClient) MoveVMDisk(ctx context.Context, node string, vmid int, req *proxmox.MoveVMDiskRequest) (string, error) {
	if err := c.verifyVM(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.MoveVMDisk(ctx, node, vmid, req)
}

func (c *poolRestrictedClient) DeleteVM(ctx context.Context, node string, vmid int, purge bool) (string, error) {
	if err := c.verifyVM(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.DeleteVM(ctx, node, vmid, purge)
}

func (c *poolRestrictedClient) CreateVMSnapshot(ctx context.Context, node string, vmid int, req proxmox.CreateVMSnapshotRequest) (string, error) {
	if err := c.verifyVM(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.CreateVMSnapshot(ctx, node, vmid, req)
}

func (c *poolRestrictedClient) RollbackVMSnapshot(ctx context.Context, node string, vmid int, snapname string) (string, error) {
	if err := c.verifyVM(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.RollbackVMSnapshot(ctx, node, vmid, snapname)
}

func (c *poolRestrictedClient) DeleteVMSnapshot(ctx context.Context, node string, vmid int, snapname string) (string, error) {
	if err := c.verifyVM(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.DeleteVMSnapshot(ctx, node, vmid, snapname)
}

func (c *poolRestrictedClient) AddVMFirewallRule(ctx context.Context, node string, vmid int, req *proxmox.FirewallRuleRequest) error {
	if err := c.verifyVM(ctx, vmid); err != nil {
		return err
	}
	return c.proxmoxClient.AddVMFirewallRule(ctx, node, vmid, req)
}

func (c *poolRestrictedClient) DeleteVMFirewallRule(ctx context.Context, node string, vmid, pos int) error {
	if err := c.verifyVM(ctx, vmid); err != nil {
		return err
	}
	return c.proxmoxClient.DeleteVMFirewallRule(ctx, node, vmid, pos)
}

func (c *poolRestrictedClient) StartContainer(ctx context.Context, node string, vmid int) (string, error) {
	if err := c.verifyContainer(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.StartContainer(ctx, node, vmid)
}

func (c *poolRestrictedClient) StopContainer(ctx context.Context, node string, vmid int) (string, error) {
	if err := c.verifyContainer(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.StopContainer(ctx, node, vmid)
}

func (c *poolRestrictedClient) ShutdownContainer(ctx context.Context, node string, vmid int) (string, error) {
	if err := c.verifyContainer(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.ShutdownContainer(ctx, node, vmid)
}

func (c *poolRestrictedClient) RebootContainer(ctx context.Context, node string, vmid int) (string, error) {
	if err := c.verifyContainer(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.RebootContainer(ctx, node, vmid)
}

func (c *poolRestrictedClient) SetContainerConfig(ctx context.Context, node string, vmid int, req *proxmox.SetContainerConfigRequest) error {
	if err := c.verifyContainer(ctx, vmid); err != nil {
		return err
	}
	return c.proxmoxClient.SetContainerConfig(ctx, node, vmid, req)
}

func (c *poolRestrictedClient) ResizeContainerDisk(ctx context.Context, node string, vmid int, req *proxmox.ResizeDiskRequest) (string, error) {
	if err := c.verifyContainer(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.ResizeContainerDisk(ctx, node, vmid, req)
}

func (c *poolRestrictedClient) MigrateContainer(ctx context.Context, node string, vmid int, req *proxmox.MigrateContainerRequest) (string, error) {
	if err := c.verifyContainer(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.MigrateContainer(ctx, node, vmid, req)
}

func (c *poolRestrictedClient) DeleteContainer(ctx context.Context, node string, vmid int, purge bool) (string, error) {
	if err := c.verifyContainer(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.DeleteContainer(ctx, node, vmid, purge)
}

func (c *poolRestrictedClient) CreateContainerSnapshot(ctx context.Context, node string, vmid int, req proxmox.CreateContainerSnapshotRequest) (string, error) {
	if err := c.verifyContainer(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.CreateContainerSnapshot(ctx, node, vmid, req)
}

func (c *poolRestrictedClient) RollbackContainerSnapshot(ctx context.Context, node string, vmid int, snapname string) (string, error) {
	if err := c.verifyContainer(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.RollbackContainerSnapshot(ctx, node, vmid, snapname)
}

func (c *poolRestrictedClient) DeleteContainerSnapshot(ctx context.Context, node string, vmid int, snapname string) (string, error) {
	if err := c.verifyContainer(ctx, vmid); err != nil {
		return "", err
	}
	return c.proxmoxClient.DeleteContainerSnapshot(ctx, node, vmid, snapname)
}

func (c *poolRestrictedClient) AddContainerFirewallRule(ctx context.Context, node string, vmid int, req *proxmox.FirewallRuleRequest) error {
	if err := c.verifyContainer(ctx, vmid); err != nil {
		return err
	}
	return c.proxmoxClient.AddContainerFirewallRule(ctx, node, vmid, req)
}

func (c *poolRestrictedClient) DeleteContainerFirewallRule(ctx context.Context, node string, vmid, pos int) error {
	if err := c.verifyContainer(ctx, vmid); err != nil {
		return err
	}
	return c.proxmoxClient.DeleteContainerFirewallRule(ctx, node, vmid, pos)
}
