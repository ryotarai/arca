package machine

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/ryotarai/arca/internal/db"
)

type fakeGceComputeClient struct {
	instances map[string]*gceInstance
	ops       map[string]*gceOperation

	inserted []*gceInsertInstanceRequest
	started  []string
	stopped  []string
	deleted  []string
}

func newFakeGceComputeClient() *fakeGceComputeClient {
	return &fakeGceComputeClient{
		instances: map[string]*gceInstance{},
		ops:       map[string]*gceOperation{},
	}
}

func (f *fakeGceComputeClient) GetInstance(_ context.Context, _, _, instance string) (*gceInstance, error) {
	item, ok := f.instances[instance]
	if !ok {
		return nil, &gceAPIError{StatusCode: 404, Message: "not found"}
	}
	return item, nil
}

func (f *fakeGceComputeClient) InsertInstance(_ context.Context, _, _ string, instance *gceInsertInstanceRequest) (*gceOperation, error) {
	f.inserted = append(f.inserted, instance)
	if f.instances == nil {
		f.instances = map[string]*gceInstance{}
	}
	f.instances[instance.Name] = &gceInstance{Name: instance.Name, Status: "RUNNING", NetworkInterfaces: []struct {
		NetworkIP string `json:"networkIP"`
	}{{NetworkIP: "10.0.0.10"}}}
	return &gceOperation{Name: "insert-op"}, nil
}

func (f *fakeGceComputeClient) StartInstance(_ context.Context, _, _, instance string) (*gceOperation, error) {
	f.started = append(f.started, instance)
	if current, ok := f.instances[instance]; ok {
		current.Status = "RUNNING"
		if len(current.NetworkInterfaces) == 0 {
			current.NetworkInterfaces = []struct {
				NetworkIP string `json:"networkIP"`
			}{{NetworkIP: "10.0.0.10"}}
		}
	}
	return &gceOperation{Name: "start-op"}, nil
}

func (f *fakeGceComputeClient) StopInstance(_ context.Context, _, _, instance string) (*gceOperation, error) {
	f.stopped = append(f.stopped, instance)
	if current, ok := f.instances[instance]; ok {
		current.Status = "TERMINATED"
	}
	return &gceOperation{Name: "stop-op"}, nil
}

func (f *fakeGceComputeClient) DeleteInstance(_ context.Context, _, _, instance string) (*gceOperation, error) {
	f.deleted = append(f.deleted, instance)
	if _, ok := f.instances[instance]; !ok {
		return nil, &gceAPIError{StatusCode: 404, Message: "not found"}
	}
	delete(f.instances, instance)
	return &gceOperation{Name: "delete-op"}, nil
}

func (f *fakeGceComputeClient) WaitZoneOperation(_ context.Context, _, _, operation string) (*gceOperation, error) {
	if op, ok := f.ops[operation]; ok {
		return op, nil
	}
	return &gceOperation{Name: operation, Status: "DONE"}, nil
}

func TestGceRuntime_EnsureRunningCreatesInstanceWhenMissing(t *testing.T) {
	t.Parallel()

	fakeClient := newFakeGceComputeClient()
	runtime, err := NewGceRuntimeWithOptions(GceRuntimeOptions{
		Project:             "project-a",
		Zone:                "us-central1-a",
		Network:             "main",
		Subnetwork:          "main-subnet",
		ServiceAccountEmail: "svc@example.iam.gserviceaccount.com",
		StartupScript:       "echo startup from gce",
		Client: fakeClient,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	machine := db.Machine{ID: "machine-123456789abc", RuntimeID: "rt-gce"}
	instanceID, err := runtime.EnsureRunning(context.Background(), machine, RuntimeStartOptions{})
	if err != nil {
		t.Fatalf("ensure running: %v", err)
	}
	if instanceID != "arca-machine-machine-1234" {
		t.Fatalf("instance id = %q", instanceID)
	}
	if len(fakeClient.inserted) != 1 {
		t.Fatalf("insert calls = %d, want 1", len(fakeClient.inserted))
	}
	inserted := fakeClient.inserted[0]
	if inserted.Name != instanceID {
		t.Fatalf("inserted instance name = %q, want %q", inserted.Name, instanceID)
	}
	if inserted.MachineType != "zones/us-central1-a/machineTypes/e2-standard-2" {
		t.Fatalf("machine type = %q", inserted.MachineType)
	}
	if len(inserted.Metadata.Items) == 0 || inserted.Metadata.Items[0].Key != "user-data" {
		t.Fatalf("user-data metadata item is missing")
	}
	userData := inserted.Metadata.Items[0].Value
	startupScript, ok := cloudInitFileContent(userData, "/usr/local/bin/arca-user-startup.sh")
	if !ok {
		t.Fatalf("user-data metadata does not include startup script file")
	}
	if !strings.Contains(startupScript, "echo startup from gce") {
		t.Fatalf("startup script content is not propagated in user-data")
	}
}

func TestGceRuntime_EnsureRunningStartsTerminatedInstance(t *testing.T) {
	t.Parallel()

	fakeClient := newFakeGceComputeClient()
	fakeClient.instances["instance-a"] = &gceInstance{Name: "instance-a", Status: "TERMINATED"}

	runtime, err := NewGceRuntimeWithOptions(GceRuntimeOptions{
		Project:             "project-a",
		Zone:                "us-central1-a",
		Network:             "main",
		Subnetwork:          "main-subnet",
		ServiceAccountEmail: "svc@example.iam.gserviceaccount.com",
		Client:              fakeClient,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	machine := db.Machine{ID: "machine-ignored", ContainerID: "instance-a"}
	instanceID, err := runtime.EnsureRunning(context.Background(), machine, RuntimeStartOptions{})
	if err != nil {
		t.Fatalf("ensure running: %v", err)
	}
	if instanceID != "instance-a" {
		t.Fatalf("instance id = %q", instanceID)
	}
	if !reflect.DeepEqual(fakeClient.started, []string{"instance-a"}) {
		t.Fatalf("start calls = %#v", fakeClient.started)
	}
}

func TestGceRuntime_EnsureStoppedStopsRunningInstance(t *testing.T) {
	t.Parallel()

	fakeClient := newFakeGceComputeClient()
	fakeClient.instances["instance-a"] = &gceInstance{Name: "instance-a", Status: "RUNNING"}

	runtime, err := NewGceRuntimeWithOptions(GceRuntimeOptions{
		Project:             "project-a",
		Zone:                "us-central1-a",
		Network:             "main",
		Subnetwork:          "main-subnet",
		ServiceAccountEmail: "svc@example.iam.gserviceaccount.com",
		Client:              fakeClient,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	if err := runtime.EnsureStopped(context.Background(), db.Machine{ID: "machine", ContainerID: "instance-a"}); err != nil {
		t.Fatalf("ensure stopped: %v", err)
	}
	if !reflect.DeepEqual(fakeClient.stopped, []string{"instance-a"}) {
		t.Fatalf("stop calls = %#v", fakeClient.stopped)
	}
}

func TestGceRuntime_IsRunningHandlesMissingInstance(t *testing.T) {
	t.Parallel()

	fakeClient := newFakeGceComputeClient()
	runtime, err := NewGceRuntimeWithOptions(GceRuntimeOptions{
		Project:             "project-a",
		Zone:                "us-central1-a",
		Network:             "main",
		Subnetwork:          "main-subnet",
		ServiceAccountEmail: "svc@example.iam.gserviceaccount.com",
		Client:              fakeClient,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	running, instanceID, err := runtime.IsRunning(context.Background(), db.Machine{ID: "abc"})
	if err != nil {
		t.Fatalf("is running: %v", err)
	}
	if running {
		t.Fatalf("running = true, want false")
	}
	if !strings.HasPrefix(instanceID, "arca-machine-") {
		t.Fatalf("instance id = %q", instanceID)
	}
}

func TestRegionFromZone(t *testing.T) {
	t.Parallel()

	if got := regionFromZone("us-central1-a"); got != "us-central1" {
		t.Fatalf("region from zone = %q", got)
	}
	if got := regionFromZone("invalid"); got != "" {
		t.Fatalf("region from invalid = %q", got)
	}
}

func TestGceSubnetworkPath(t *testing.T) {
	t.Parallel()

	got := gceSubnetworkPath("project-a", "us-central1-a", "subnet-main")
	want := "projects/project-a/regions/us-central1/subnetworks/subnet-main"
	if got != want {
		t.Fatalf("subnetwork path = %q, want %q", got, want)
	}

	selfLink := "https://www.googleapis.com/compute/v1/projects/project-a/regions/us-central1/subnetworks/subnet-main"
	if got = gceSubnetworkPath("project-a", "us-central1-a", selfLink); got != selfLink {
		t.Fatalf("subnetwork self-link path = %q, want %q", got, selfLink)
	}
}

func TestGceRuntime_NewValidatesRequiredFields(t *testing.T) {
	t.Parallel()

	_, err := NewGceRuntimeWithOptions(GceRuntimeOptions{})
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(err.Error(), "requires project") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGceRuntime_EnsureDeletedDeletesInstance(t *testing.T) {
	t.Parallel()

	fakeClient := newFakeGceComputeClient()
	fakeClient.instances["instance-a"] = &gceInstance{Name: "instance-a", Status: "TERMINATED"}

	runtime, err := NewGceRuntimeWithOptions(GceRuntimeOptions{
		Project:             "project-a",
		Zone:                "us-central1-a",
		Network:             "main",
		Subnetwork:          "main-subnet",
		ServiceAccountEmail: "svc@example.iam.gserviceaccount.com",
		Client:              fakeClient,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	if err := runtime.EnsureDeleted(context.Background(), db.Machine{ID: "machine", ContainerID: "instance-a"}); err != nil {
		t.Fatalf("ensure deleted: %v", err)
	}
	if !reflect.DeepEqual(fakeClient.deleted, []string{"instance-a"}) {
		t.Fatalf("delete calls = %#v", fakeClient.deleted)
	}
}

func TestGceRuntime_EnsureDeletedMissingInstanceIsNoop(t *testing.T) {
	t.Parallel()

	fakeClient := newFakeGceComputeClient()
	runtime, err := NewGceRuntimeWithOptions(GceRuntimeOptions{
		Project:             "project-a",
		Zone:                "us-central1-a",
		Network:             "main",
		Subnetwork:          "main-subnet",
		ServiceAccountEmail: "svc@example.iam.gserviceaccount.com",
		Client:              fakeClient,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	if err := runtime.EnsureDeleted(context.Background(), db.Machine{ID: "machine", ContainerID: "instance-missing"}); err != nil {
		t.Fatalf("ensure deleted on missing instance: %v", err)
	}
	if !reflect.DeepEqual(fakeClient.deleted, []string{"instance-missing"}) {
		t.Fatalf("delete calls = %#v", fakeClient.deleted)
	}
}
