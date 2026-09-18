package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gordcurrie/proxmox-mcp/internal/proxmox"
)

func TestPoolRestrictionDisabledPreservesClient(t *testing.T) {
	called := false
	mock := &mockProxmoxClient{createVMFn: func(_ context.Context, _ string, _ *proxmox.CreateVMRequest) (string, error) {
		called = true
		return "upid", nil
	}}
	got, err := mock.CreateVM(context.Background(), "node", &proxmox.CreateVMRequest{VMID: 1})
	if err != nil || got != "upid" || !called {
		t.Fatalf("unrestricted client changed behavior: got %q, %v, called=%v", got, err, called)
	}
}

func TestPoolRestrictionCreateVM(t *testing.T) {
	t.Run("allowed pool is forwarded", func(t *testing.T) {
		var got *proxmox.CreateVMRequest
		client := &poolRestrictedClient{
			proxmoxClient: &mockProxmoxClient{createVMFn: func(_ context.Context, _ string, req *proxmox.CreateVMRequest) (string, error) {
				got = req
				return "upid", nil
			}},
			allowedPool: "test-pool",
		}
		if _, err := client.CreateVM(context.Background(), "node", &proxmox.CreateVMRequest{Pool: "test-pool"}); err != nil {
			t.Fatal(err)
		}
		if got == nil || got.Pool != "test-pool" {
			t.Fatalf("request pool was not forwarded: %#v", got)
		}
	})

	for _, tc := range []struct {
		name string
		pool string
		want string
	}{
		{name: "different pool", pool: "other-pool", want: "not allowed"},
		{name: "omitted pool", pool: "", want: "explicitly specified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			client := &poolRestrictedClient{
				proxmoxClient: &mockProxmoxClient{createVMFn: func(_ context.Context, _ string, _ *proxmox.CreateVMRequest) (string, error) {
					called = true
					return "", nil
				}},
				allowedPool: "test-pool",
			}
			_, err := client.CreateVM(context.Background(), "node", &proxmox.CreateVMRequest{Pool: tc.pool})
			if err == nil || !strings.Contains(err.Error(), tc.want) || called {
				t.Fatalf("got err=%v called=%v", err, called)
			}
		})
	}
}

func TestPoolRestrictionListPoolsUsesExplicitLookup(t *testing.T) {
	calls := 0
	client := &poolRestrictedClient{
		proxmoxClient: &mockProxmoxClient{
			listPoolsFn: func(context.Context) ([]proxmox.Pool, error) {
				t.Fatal("unfiltered list_pools must not be called")
				return nil, nil
			},
			getPoolFn: func(_ context.Context, poolid string) (*proxmox.Pool, error) {
				calls++
				return &proxmox.Pool{PoolID: poolid}, nil
			},
		},
		allowedPool: "test-pool",
	}
	pools, err := client.ListPools(context.Background())
	if err != nil || calls != 1 || len(pools) != 1 || pools[0].PoolID != "test-pool" {
		t.Fatalf("unexpected result: pools=%#v err=%v calls=%d", pools, err, calls)
	}
}

func TestPoolRestrictionMembership(t *testing.T) {
	base := func(called *bool) *poolRestrictedClient {
		return &poolRestrictedClient{
			proxmoxClient: &mockProxmoxClient{
				getPoolFn: func(_ context.Context, poolid string) (*proxmox.Pool, error) {
					return &proxmox.Pool{PoolID: poolid, Members: []proxmox.PoolMember{{ID: "qemu/100", Type: "qemu", VMID: 100}}}, nil
				},
				startVMFn: func(context.Context, string, int) (string, error) {
					*called = true
					return "upid", nil
				},
			},
			allowedPool: "test-pool",
		}
	}

	t.Run("member is allowed", func(t *testing.T) {
		called := false
		got, err := base(&called).StartVM(context.Background(), "node", 100)
		if err != nil || got != "upid" || !called {
			t.Fatalf("got %q, %v, called=%v", got, err, called)
		}
	})

	t.Run("outside member is rejected before mutation", func(t *testing.T) {
		called := false
		_, err := base(&called).StartVM(context.Background(), "node", 101)
		if err == nil || !strings.Contains(err.Error(), "not a member") || called {
			t.Fatalf("got err=%v called=%v", err, called)
		}
	})

	t.Run("membership lookup failure fails closed", func(t *testing.T) {
		called := false
		client := base(&called)
		client.proxmoxClient = &mockProxmoxClient{
			getPoolFn: func(context.Context, string) (*proxmox.Pool, error) { return nil, errors.New("permission denied") },
			startVMFn: func(context.Context, string, int) (string, error) { called = true; return "upid", nil },
		}
		_, err := client.StartVM(context.Background(), "node", 100)
		if err == nil || !strings.Contains(err.Error(), "permission denied") || called {
			t.Fatalf("got err=%v called=%v", err, called)
		}
	})
}

func TestPoolRestrictionCloneCannotEscape(t *testing.T) {
	called := false
	client := &poolRestrictedClient{
		proxmoxClient: &mockProxmoxClient{
			getPoolFn: func(context.Context, string) (*proxmox.Pool, error) {
				return &proxmox.Pool{Members: []proxmox.PoolMember{{Type: "qemu", VMID: 100}}}, nil
			},
			cloneVMFn: func(context.Context, string, int, *proxmox.CloneVMRequest) (string, error) {
				called = true
				return "upid", nil
			},
		},
		allowedPool: "test-pool",
	}
	_, err := client.CloneVM(context.Background(), "node", 100, &proxmox.CloneVMRequest{NewID: 101})
	if err == nil || !strings.Contains(err.Error(), "cannot explicitly bind") || called {
		t.Fatalf("got err=%v called=%v", err, called)
	}
}

func TestPoolRestrictionContainerOperations(t *testing.T) {
	called := false
	client := &poolRestrictedClient{
		proxmoxClient: &mockProxmoxClient{
			getPoolFn: func(context.Context, string) (*proxmox.Pool, error) {
				return &proxmox.Pool{Members: []proxmox.PoolMember{{Type: "lxc", VMID: 200}}}, nil
			},
			startContainerFn: func(context.Context, string, int) (string, error) { called = true; return "upid", nil },
		},
		allowedPool: "test-pool",
	}
	if _, err := client.StartContainer(context.Background(), "node", 200); err != nil || !called {
		t.Fatalf("allowed container operation failed: err=%v called=%v", err, called)
	}

	called = false
	if _, err := client.StartContainer(context.Background(), "node", 201); err == nil || called {
		t.Fatalf("outside container operation was not rejected: err=%v called=%v", err, called)
	}
}

func TestPoolRestrictionRestoreCannotBindDestination(t *testing.T) {
	client := &poolRestrictedClient{proxmoxClient: &mockProxmoxClient{}, allowedPool: "test-pool"}
	if _, err := client.RestoreVM(context.Background(), "node", &proxmox.RestoreVMRequest{}); err == nil || !strings.Contains(err.Error(), "cannot explicitly bind") {
		t.Fatalf("restore was not rejected: %v", err)
	}
	if _, err := client.CreateContainer(context.Background(), "node", &proxmox.CreateContainerRequest{}); err == nil || !strings.Contains(err.Error(), "cannot explicitly bind") {
		t.Fatalf("container creation was not rejected: %v", err)
	}
}

func TestPoolRestrictionBackupRequiresMembership(t *testing.T) {
	called := false
	client := &poolRestrictedClient{
		proxmoxClient: &mockProxmoxClient{
			getPoolFn: func(context.Context, string) (*proxmox.Pool, error) {
				return &proxmox.Pool{Members: []proxmox.PoolMember{{Type: "qemu", VMID: 100}}}, nil
			},
			createBackupFn: func(context.Context, string, *proxmox.CreateBackupRequest) (string, error) {
				called = true
				return "upid", nil
			},
		},
		allowedPool: "test-pool",
	}
	if _, err := client.CreateBackup(context.Background(), "node", &proxmox.CreateBackupRequest{VMID: 101}); err == nil || called {
		t.Fatalf("backup outside pool was not rejected: err=%v called=%v", err, called)
	}
	if _, err := client.CreateBackup(context.Background(), "node", &proxmox.CreateBackupRequest{VMID: 100}); err != nil || !called {
		t.Fatalf("backup for allowed VM failed: err=%v called=%v", err, called)
	}
}

func TestPoolRestrictionRejectsAllPoolMutations(t *testing.T) {
	called := false
	client := &poolRestrictedClient{
		proxmoxClient: &mockProxmoxClient{
			createPoolFn: func(context.Context, *proxmox.CreatePoolRequest) error { called = true; return nil },
			updatePoolFn: func(context.Context, string, *proxmox.UpdatePoolRequest) error { called = true; return nil },
			deletePoolFn: func(context.Context, string) error { called = true; return nil },
		},
		allowedPool: "test-pool",
	}
	if err := client.CreatePool(context.Background(), &proxmox.CreatePoolRequest{PoolID: "other-pool"}); err == nil || called {
		t.Fatalf("create_pool was not rejected: err=%v called=%v", err, called)
	}
	if err := client.UpdatePool(context.Background(), "test-pool", &proxmox.UpdatePoolRequest{}); err == nil || called {
		t.Fatalf("update_pool was not rejected: err=%v called=%v", err, called)
	}
	if err := client.DeletePool(context.Background(), "test-pool"); err == nil || called {
		t.Fatalf("delete_pool was not rejected: err=%v called=%v", err, called)
	}
}
