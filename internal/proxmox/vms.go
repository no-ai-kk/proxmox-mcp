package proxmox

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ListVMs returns all QEMU virtual machines on the specified node.
func (c *Client) ListVMs(ctx context.Context, node string) ([]VM, error) {
	var vms []VM
	if err := c.get(ctx, "/nodes/"+url.PathEscape(node)+"/qemu", &vms); err != nil {
		return nil, fmt.Errorf("listing VMs on node %s: %w", node, err)
	}
	return vms, nil
}

// GetVMStatus returns detailed status and configuration for a specific VM.
// The returned map contains the full Proxmox API response for
// /nodes/{node}/qemu/{vmid}/status/current.
func (c *Client) GetVMStatus(ctx context.Context, node string, vmid int) (map[string]any, error) {
	var status map[string]any
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/status/current"
	if err := c.get(ctx, path, &status); err != nil {
		return nil, fmt.Errorf("getting status for VM %d on node %s: %w", vmid, node, err)
	}
	return status, nil
}

// GetVMGuestNetworkInterfaces returns a compact representation of the network
// interfaces reported by the QEMU guest agent. The call performs one query and
// returns immediately if the VM or guest agent is not ready.
func (c *Client) GetVMGuestNetworkInterfaces(ctx context.Context, node string, vmid int) ([]GuestNetworkInterface, error) {
	type agentIPAddress struct {
		Address string `json:"ip-address"`
		Family  string `json:"ip-address-type"`
		Prefix  int    `json:"prefix"`
	}
	type agentInterface struct {
		Name        string           `json:"name"`
		MACAddress  string           `json:"hardware-address"`
		IPAddresses []agentIPAddress `json:"ip-addresses"`
	}
	var response struct {
		Result []agentInterface `json:"result"`
	}
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/agent/network-get-interfaces"
	if err := c.get(ctx, path, &response); err != nil {
		return nil, fmt.Errorf("getting guest network interfaces for VM %d on node %s: %w", vmid, node, err)
	}

	interfaces := make([]GuestNetworkInterface, 0, len(response.Result))
	for _, iface := range response.Result {
		addresses := make([]GuestIPAddress, 0, len(iface.IPAddresses))
		for _, address := range iface.IPAddresses {
			addresses = append(addresses, GuestIPAddress{
				Address:      address.Address,
				Family:       address.Family,
				PrefixLength: address.Prefix,
			})
		}
		interfaces = append(interfaces, GuestNetworkInterface{
			Name:        iface.Name,
			MACAddress:  iface.MACAddress,
			IPAddresses: addresses,
		})
	}
	return interfaces, nil
}

// StartVM starts a QEMU VM. It returns the UPID of the asynchronous task.
// The task completes asynchronously; use GetTaskStatus to poll for completion.
func (c *Client) StartVM(ctx context.Context, node string, vmid int) (string, error) {
	var upid string
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/status/start"
	if err := c.post(ctx, path, &upid); err != nil {
		return "", fmt.Errorf("starting VM %d on node %s: %w", vmid, node, err)
	}
	return upid, nil
}

// StopVM performs a hard stop of a QEMU VM. It returns the UPID of the
// asynchronous task.
func (c *Client) StopVM(ctx context.Context, node string, vmid int) (string, error) {
	var upid string
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/status/stop"
	if err := c.post(ctx, path, &upid); err != nil {
		return "", fmt.Errorf("stopping VM %d on node %s: %w", vmid, node, err)
	}
	return upid, nil
}

// ShutdownVM sends a graceful ACPI shutdown signal to a QEMU VM.
// It returns the UPID of the asynchronous task.
func (c *Client) ShutdownVM(ctx context.Context, node string, vmid int) (string, error) {
	var upid string
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/status/shutdown"
	if err := c.post(ctx, path, &upid); err != nil {
		return "", fmt.Errorf("shutting down VM %d on node %s: %w", vmid, node, err)
	}
	return upid, nil
}

// RebootVM reboots a QEMU VM. It returns the UPID of the asynchronous task.
func (c *Client) RebootVM(ctx context.Context, node string, vmid int) (string, error) {
	var upid string
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/status/reboot"
	if err := c.post(ctx, path, &upid); err != nil {
		return "", fmt.Errorf("rebooting VM %d on node %s: %w", vmid, node, err)
	}
	return upid, nil
}

// SuspendVM suspends a QEMU VM. It returns the UPID of the asynchronous task.
func (c *Client) SuspendVM(ctx context.Context, node string, vmid int) (string, error) {
	var upid string
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/status/suspend"
	if err := c.post(ctx, path, &upid); err != nil {
		return "", fmt.Errorf("suspending VM %d on node %s: %w", vmid, node, err)
	}
	return upid, nil
}

// ResumeVM resumes a suspended QEMU VM. It returns the UPID of the asynchronous task.
func (c *Client) ResumeVM(ctx context.Context, node string, vmid int) (string, error) {
	var upid string
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/status/resume"
	if err := c.post(ctx, path, &upid); err != nil {
		return "", fmt.Errorf("resuming VM %d on node %s: %w", vmid, node, err)
	}
	return upid, nil
}

// DeleteVM deletes a QEMU VM. It returns the UPID of the asynchronous task.
// If purge is true, associated disk images are also removed.
// The VM must be stopped before deletion.
func (c *Client) DeleteVM(ctx context.Context, node string, vmid int, purge bool) (string, error) {
	var upid string
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid)
	if purge {
		path += "?purge=1"
	}
	if err := c.delete(ctx, path, &upid); err != nil {
		return "", fmt.Errorf("deleting VM %d on node %s: %w", vmid, node, err)
	}
	return upid, nil
}

// CreateVM creates a new QEMU VM on the specified node. It returns the UPID
// of the asynchronous task.
func (c *Client) CreateVM(ctx context.Context, node string, req *CreateVMRequest) (string, error) {
	if req == nil {
		return "", errors.New("CreateVM: req must not be nil")
	}
	var upid string
	path := "/nodes/" + url.PathEscape(node) + "/qemu"
	if err := c.postWithBody(ctx, path, req, &upid); err != nil {
		return "", fmt.Errorf("creating VM %d on node %s: %w", req.VMID, node, err)
	}
	return upid, nil
}

// CloneVM clones an existing QEMU VM to a new VM ID. It returns the UPID
// of the asynchronous task.
func (c *Client) CloneVM(ctx context.Context, node string, vmid int, req *CloneVMRequest) (string, error) {
	if req == nil {
		return "", errors.New("CloneVM: req must not be nil")
	}
	var upid string
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/clone"
	if err := c.postWithBody(ctx, path, req, &upid); err != nil {
		return "", fmt.Errorf("cloning VM %d on node %s: %w", vmid, node, err)
	}
	return upid, nil
}

// GetVMConfig returns the full configuration for a QEMU VM.
func (c *Client) GetVMConfig(ctx context.Context, node string, vmid int) (map[string]any, error) {
	var config map[string]any
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/config"
	if err := c.get(ctx, path, &config); err != nil {
		return nil, fmt.Errorf("getting config for VM %d on node %s: %w", vmid, node, err)
	}
	return config, nil
}

// SetVMConfig updates the configuration of a QEMU VM synchronously via PUT.
// Only fields set on req are sent; zero-value fields are omitted. No task is
// returned — the change takes effect immediately.
func (c *Client) SetVMConfig(ctx context.Context, node string, vmid int, req *SetVMConfigRequest) error {
	if req == nil {
		return errors.New("SetVMConfig: req must not be nil")
	}
	var result any
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/config"
	if err := c.put(ctx, path, req, &result); err != nil {
		return fmt.Errorf("setting config for VM %d on node %s: %w", vmid, node, err)
	}
	return nil
}

// SetVMCloudInit configures the supported minimal cloud-init fields for a VM
// synchronously via the standard VM config endpoint.
func (c *Client) SetVMCloudInit(ctx context.Context, node string, vmid int, req *SetVMCloudInitRequest) error {
	if req == nil {
		return errors.New("SetVMCloudInit: req must not be nil")
	}
	request := *req
	request.SSHKeys = encodeProxmoxSSHKeys(request.SSHKeys)
	var result any
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/config"
	if err := c.put(ctx, path, &request, &result); err != nil {
		return fmt.Errorf("setting cloud-init config for VM %d on node %s: %w", vmid, node, err)
	}
	return nil
}

// encodeProxmoxSSHKeys prepares the raw OpenSSH key value for Proxmox's
// urlencoded VM config parameter. QueryEscape is used for its percent-escape
// rules, but its form-encoding of spaces as '+' is not accepted by Proxmox.
func encodeProxmoxSSHKeys(keys string) string {
	return strings.ReplaceAll(url.QueryEscape(strings.TrimSpace(keys)), "+", "%20")
}

// ResizeVMDisk resizes a disk attached to a QEMU VM. It returns the UPID of
// the asynchronous task. Size may be absolute (e.g. "50G") or a relative
// increment (e.g. "+10G").
func (c *Client) ResizeVMDisk(ctx context.Context, node string, vmid int, req *ResizeDiskRequest) (string, error) {
	if req == nil {
		return "", errors.New("ResizeVMDisk: req must not be nil")
	}
	var upid string
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/resize"
	if err := c.put(ctx, path, req, &upid); err != nil {
		return "", fmt.Errorf("resizing disk %s on VM %d on node %s: %w", req.Disk, vmid, node, err)
	}
	return upid, nil
}

// RestoreVM restores a QEMU VM from a vzdump backup archive. It returns the
// UPID of the asynchronous task. The archive field must be a full volid such
// as "local:backup/vzdump-qemu-100-....vma.zst". Supply vmid even if the
// backup was taken of a different ID — Proxmox assigns the new ID from the
// request. Set req.Start = 1 to start the VM automatically after restore.
func (c *Client) RestoreVM(ctx context.Context, node string, req *RestoreVMRequest) (string, error) {
	if req == nil {
		return "", errors.New("RestoreVM: req must not be nil")
	}
	var upid string
	path := "/nodes/" + url.PathEscape(node) + "/qemu"
	if err := c.postWithBody(ctx, path, req, &upid); err != nil {
		return "", fmt.Errorf("restoring VM %d on node %s: %w", req.VMID, node, err)
	}
	return upid, nil
}

// MigrateVM migrates a QEMU VM to another node. It returns the UPID of the
// asynchronous task. Set req.Online = true for live migration (no guest
// downtime when QEMU guest agent is running).
func (c *Client) MigrateVM(ctx context.Context, node string, vmid int, req *MigrateVMRequest) (string, error) {
	if req == nil {
		return "", errors.New("MigrateVM: req must not be nil")
	}
	var upid string
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/migrate"
	if err := c.postWithBody(ctx, path, req, &upid); err != nil {
		return "", fmt.Errorf("migrating VM %d from node %s to %s: %w", vmid, node, req.Target, err)
	}
	return upid, nil
}

// MoveVMDisk moves a disk attached to a QEMU VM to a different storage pool.
// It returns the UPID of the asynchronous task. If req.DeleteSource is set to 1
// the source volume is removed after the move completes successfully.
func (c *Client) MoveVMDisk(ctx context.Context, node string, vmid int, req *MoveVMDiskRequest) (string, error) {
	if req == nil {
		return "", errors.New("MoveVMDisk: req must not be nil")
	}
	var upid string
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/move_disk"
	if err := c.postWithBody(ctx, path, req, &upid); err != nil {
		return "", fmt.Errorf("moving disk %s on VM %d on node %s: %w", req.Disk, vmid, node, err)
	}
	return upid, nil
}
