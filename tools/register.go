// Package tools registers all MCP tools onto the server.
package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Config holds optional feature flags for tool registration.
type Config struct {
	// AllowDestructive enables destructive tools (delete_vm, delete_container).
	// Corresponds to the PROXMOX_ALLOW_DESTRUCTIVE environment variable.
	// Defaults to false — destructive tools are not registered unless explicitly opted in.
	AllowDestructive bool
	// AllowedPool restricts mutating operations to members of this pool.
	// An empty value preserves the unrestricted default behavior.
	AllowedPool string
	// AllowedCloneSource permits clone_vm to use this VM as a source without
	// treating it as a member of AllowedPool. Zero means no exception.
	AllowedCloneSource int
}

// RegisterAll wires all Proxmox MCP tools onto the provided server.
func RegisterAll(s *mcp.Server, client proxmoxClient, cfg Config) {
	if cfg.AllowedPool != "" {
		client = &poolRestrictedClient{
			proxmoxClient:      client,
			allowedPool:        cfg.AllowedPool,
			allowedCloneSource: cfg.AllowedCloneSource,
		}
	}

	registerNodeTools(s, client)
	registerVMTools(s, client)
	registerContainerTools(s, client)
	registerClusterTools(s, client)
	registerSnapshotTools(s, client)
	registerStorageTools(s, client)
	registerBackupTools(s, client)
	registerNetworkTools(s, client)
	registerFirewallTools(s, client)
	registerPoolTools(s, client)
	registerStorageDefTools(s, client)
	registerAccessTools(s, client)
	if cfg.AllowDestructive {
		registerDestructiveTools(s, client)
	}
}
