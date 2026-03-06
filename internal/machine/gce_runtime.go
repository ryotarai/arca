package machine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2/google"

	"github.com/ryotarai/arca/internal/db"
)

const (
	defaultGceMachineType        = "e2-standard-2"
	defaultGceDiskSizeGB   int64 = 40
	defaultGceImageProject       = "ubuntu-os-cloud"
	defaultGceImageFamily        = "ubuntu-2404-lts-amd64"
	defaultGceArcadGOOS          = "linux"
	defaultGceArcadGOARCH        = "amd64"
)

type GceRuntime struct {
	project             string
	zone                string
	network             string
	subnetwork          string
	serviceAccountEmail string
	machineType         string
	diskSizeGB          int64
	imageProject        string
	imageFamily         string
	arcadGOOS           string
	arcadGOARCH         string

	clientFactory        func(context.Context) (gceComputeClient, error)
	buildArcadBinaryBase func(context.Context) (string, error)
	waitReadyHTTP        func(context.Context, string) error

	mu     sync.Mutex
	client gceComputeClient
}

type GceRuntimeOptions struct {
	Project             string
	Zone                string
	Network             string
	Subnetwork          string
	ServiceAccountEmail string
	MachineType         string
	DiskSizeGB          int64
	ImageProject        string
	ImageFamily         string
	ArcadGOOS           string
	ArcadGOARCH         string

	Client               gceComputeClient
	ClientFactory        func(context.Context) (gceComputeClient, error)
	BuildArcadBinaryBase func(context.Context) (string, error)
	WaitReadyHTTP        func(context.Context, string) error
}

type gceComputeClient interface {
	GetInstance(context.Context, string, string, string) (*gceInstance, error)
	InsertInstance(context.Context, string, string, *gceInsertInstanceRequest) (*gceOperation, error)
	StartInstance(context.Context, string, string, string) (*gceOperation, error)
	StopInstance(context.Context, string, string, string) (*gceOperation, error)
	DeleteInstance(context.Context, string, string, string) (*gceOperation, error)
	WaitZoneOperation(context.Context, string, string, string) (*gceOperation, error)
}

type gceRESTClient struct {
	httpClient *http.Client
}

type gceInstance struct {
	Name              string `json:"name"`
	Status            string `json:"status"`
	NetworkInterfaces []struct {
		NetworkIP string `json:"networkIP"`
	} `json:"networkInterfaces"`
}

type gceOperation struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  *struct {
		Errors []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	} `json:"error"`
}

type gceInsertInstanceRequest struct {
	Name        string `json:"name"`
	MachineType string `json:"machineType"`
	Disks       []struct {
		AutoDelete       bool   `json:"autoDelete"`
		Boot             bool   `json:"boot"`
		Type             string `json:"type"`
		InitializeParams struct {
			SourceImage string `json:"sourceImage"`
			DiskSizeGb  int64  `json:"diskSizeGb"`
		} `json:"initializeParams"`
	} `json:"disks"`
	NetworkInterfaces []struct {
		Network    string `json:"network"`
		Subnetwork string `json:"subnetwork"`
	} `json:"networkInterfaces"`
	ServiceAccounts []struct {
		Email  string   `json:"email"`
		Scopes []string `json:"scopes"`
	} `json:"serviceAccounts"`
	Metadata struct {
		Items []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"items"`
	} `json:"metadata"`
}

type gceAPIError struct {
	StatusCode int
	Message    string
}

func (e *gceAPIError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) == "" {
		return fmt.Sprintf("gce api request failed with status %d", e.StatusCode)
	}
	return fmt.Sprintf("gce api request failed with status %d: %s", e.StatusCode, e.Message)
}

func NewGceRuntimeWithOptions(options GceRuntimeOptions) (*GceRuntime, error) {
	project := strings.TrimSpace(options.Project)
	zone := strings.TrimSpace(options.Zone)
	network := strings.TrimSpace(options.Network)
	subnetwork := strings.TrimSpace(options.Subnetwork)
	serviceAccountEmail := strings.TrimSpace(options.ServiceAccountEmail)
	if project == "" || zone == "" || network == "" || subnetwork == "" || serviceAccountEmail == "" {
		return nil, fmt.Errorf("gce runtime config requires project, zone, network, subnetwork, and service account email")
	}

	machineType := strings.TrimSpace(options.MachineType)
	if machineType == "" {
		machineType = defaultGceMachineType
	}
	diskSizeGB := options.DiskSizeGB
	if diskSizeGB <= 0 {
		diskSizeGB = defaultGceDiskSizeGB
	}
	imageProject := strings.TrimSpace(options.ImageProject)
	if imageProject == "" {
		imageProject = defaultGceImageProject
	}
	imageFamily := strings.TrimSpace(options.ImageFamily)
	if imageFamily == "" {
		imageFamily = defaultGceImageFamily
	}
	arcadGOOS := strings.TrimSpace(options.ArcadGOOS)
	if arcadGOOS == "" {
		arcadGOOS = strings.TrimSpace(os.Getenv("ARCA_GCE_ARCAD_GOOS"))
	}
	if arcadGOOS == "" {
		arcadGOOS = defaultGceArcadGOOS
	}
	arcadGOARCH := strings.TrimSpace(options.ArcadGOARCH)
	if arcadGOARCH == "" {
		arcadGOARCH = strings.TrimSpace(os.Getenv("ARCA_GCE_ARCAD_GOARCH"))
	}
	if arcadGOARCH == "" {
		arcadGOARCH = defaultGceArcadGOARCH
	}

	runtime := &GceRuntime{
		project:              project,
		zone:                 zone,
		network:              network,
		subnetwork:           subnetwork,
		serviceAccountEmail:  serviceAccountEmail,
		machineType:          machineType,
		diskSizeGB:           diskSizeGB,
		imageProject:         imageProject,
		imageFamily:          imageFamily,
		arcadGOOS:            arcadGOOS,
		arcadGOARCH:          arcadGOARCH,
		waitReadyHTTP:        firstNonNilWaitReadyHTTP(options.WaitReadyHTTP),
		buildArcadBinaryBase: options.BuildArcadBinaryBase,
	}

	if runtime.buildArcadBinaryBase == nil {
		runtime.buildArcadBinaryBase = runtime.buildArcadBinaryBase64
	}

	switch {
	case options.Client != nil:
		runtime.client = options.Client
	case options.ClientFactory != nil:
		runtime.clientFactory = options.ClientFactory
	default:
		runtime.clientFactory = newGceRESTClient
	}

	return runtime, nil
}

func firstNonNilWaitReadyHTTP(fn func(context.Context, string) error) func(context.Context, string) error {
	if fn != nil {
		return fn
	}
	return waitHTTPReady
}

func (r *GceRuntime) EnsureRunning(ctx context.Context, machine db.Machine, opts RuntimeStartOptions) (string, error) {
	instanceName := r.instanceName(machine)
	client, err := r.computeClient(ctx)
	if err != nil {
		return "", err
	}

	instance, found, err := r.getInstance(ctx, client, instanceName)
	if err != nil {
		return "", err
	}
	if !found {
		arcadBinaryBase64, err := r.buildArcadBinaryBase(ctx)
		if err != nil {
			return "", err
		}
		cloudInit := cloudInitUserData(machine, opts, arcadBinaryBase64)
		insertOp, err := client.InsertInstance(ctx, r.project, r.zone, r.instanceSpec(instanceName, cloudInit))
		if err != nil {
			return "", fmt.Errorf("create gce instance %q: %w", instanceName, err)
		}
		if err := r.waitOperation(ctx, client, insertOp, "create"); err != nil {
			return "", err
		}
		return instanceName, nil
	}

	switch strings.ToUpper(strings.TrimSpace(instance.Status)) {
	case "RUNNING", "PROVISIONING", "STAGING":
		return instanceName, nil
	case "STOPPING":
		if err := r.waitForTerminated(ctx, client, instanceName); err != nil {
			return "", err
		}
		fallthrough
	case "TERMINATED", "SUSPENDED":
		startOp, err := client.StartInstance(ctx, r.project, r.zone, instanceName)
		if err != nil {
			return "", fmt.Errorf("start gce instance %q: %w", instanceName, err)
		}
		if err := r.waitOperation(ctx, client, startOp, "start"); err != nil {
			return "", err
		}
		return instanceName, nil
	default:
		return "", fmt.Errorf("gce instance %q has unsupported status %q", instanceName, instance.Status)
	}
}

func (r *GceRuntime) EnsureStopped(ctx context.Context, machine db.Machine) error {
	instanceName := r.instanceName(machine)
	client, err := r.computeClient(ctx)
	if err != nil {
		return err
	}

	instance, found, err := r.getInstance(ctx, client, instanceName)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	switch strings.ToUpper(strings.TrimSpace(instance.Status)) {
	case "TERMINATED", "SUSPENDED":
		return nil
	case "RUNNING", "PROVISIONING", "STAGING", "STOPPING":
		stopOp, err := client.StopInstance(ctx, r.project, r.zone, instanceName)
		if err != nil {
			if isGceNotFound(err) {
				return nil
			}
			return fmt.Errorf("stop gce instance %q: %w", instanceName, err)
		}
		return r.waitOperation(ctx, client, stopOp, "stop")
	default:
		return nil
	}
}

func (r *GceRuntime) EnsureDeleted(ctx context.Context, machine db.Machine) error {
	instanceName := r.instanceName(machine)
	client, err := r.computeClient(ctx)
	if err != nil {
		return err
	}

	deleteOp, err := client.DeleteInstance(ctx, r.project, r.zone, instanceName)
	if err != nil {
		if isGceNotFound(err) {
			return nil
		}
		return fmt.Errorf("delete gce instance %q: %w", instanceName, err)
	}
	return r.waitOperation(ctx, client, deleteOp, "delete")
}

func (r *GceRuntime) IsRunning(ctx context.Context, machine db.Machine) (bool, string, error) {
	instanceName := r.instanceName(machine)
	client, err := r.computeClient(ctx)
	if err != nil {
		return false, instanceName, err
	}

	instance, found, err := r.getInstance(ctx, client, instanceName)
	if err != nil {
		return false, instanceName, err
	}
	if !found {
		return false, instanceName, nil
	}
	status := strings.ToUpper(strings.TrimSpace(instance.Status))
	return status == "RUNNING" || status == "STAGING" || status == "PROVISIONING", instanceName, nil
}

func (r *GceRuntime) WaitReady(ctx context.Context, machine db.Machine, instanceID string) error {
	instanceName := firstNonEmpty(instanceID, machine.ContainerID, r.instanceName(machine))
	client, err := r.computeClient(ctx)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var lastErr error
	for {
		instance, found, err := r.getInstance(ctx, client, instanceName)
		if err != nil {
			lastErr = err
		} else if found {
			status := strings.ToUpper(strings.TrimSpace(instance.Status))
			if status == "RUNNING" {
				ip := gceInstanceIPv4(instance)
				if ip == "" {
					lastErr = fmt.Errorf("gce instance %q has no network ip", instanceName)
				} else {
					return r.waitReadyHTTP(ctx, fmt.Sprintf("http://%s:21030/__arca/readyz", ip))
				}
			} else {
				lastErr = fmt.Errorf("instance status is %s", status)
			}
		} else {
			lastErr = fmt.Errorf("gce instance %q not found", instanceName)
		}

		select {
		case <-ctx.Done():
			if lastErr == nil {
				return ctx.Err()
			}
			return fmt.Errorf("%w (last error: %v)", ctx.Err(), lastErr)
		case <-ticker.C:
		}
	}
}

func (r *GceRuntime) instanceName(machine db.Machine) string {
	if strings.TrimSpace(machine.ContainerID) != "" {
		return machine.ContainerID
	}
	prefix := "arca-machine-"
	if len(machine.ID) >= 12 {
		return prefix + machine.ID[:12]
	}
	if machine.ID == "" {
		return prefix + "unknown"
	}
	return prefix + machine.ID
}

func (r *GceRuntime) computeClient(ctx context.Context) (gceComputeClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.client != nil {
		return r.client, nil
	}
	if r.clientFactory == nil {
		return nil, errors.New("gce compute client is not configured")
	}
	client, err := r.clientFactory(ctx)
	if err != nil {
		return nil, fmt.Errorf("init gce compute client: %w", err)
	}
	r.client = client
	return r.client, nil
}

func (r *GceRuntime) getInstance(ctx context.Context, client gceComputeClient, instanceName string) (*gceInstance, bool, error) {
	instance, err := client.GetInstance(ctx, r.project, r.zone, instanceName)
	if err != nil {
		if isGceNotFound(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("get gce instance %q: %w", instanceName, err)
	}
	return instance, true, nil
}

func (r *GceRuntime) waitOperation(ctx context.Context, client gceComputeClient, op *gceOperation, action string) error {
	if op == nil {
		return fmt.Errorf("%s gce instance: empty operation", action)
	}
	if strings.TrimSpace(op.Name) == "" {
		return fmt.Errorf("%s gce instance: operation name is empty", action)
	}
	completed, err := client.WaitZoneOperation(ctx, r.project, r.zone, op.Name)
	if err != nil {
		return fmt.Errorf("%s gce instance wait op=%q: %w", action, op.Name, err)
	}
	if completed == nil {
		return fmt.Errorf("%s gce instance wait op=%q: empty response", action, op.Name)
	}
	if completed.Error == nil || len(completed.Error.Errors) == 0 {
		return nil
	}
	messages := make([]string, 0, len(completed.Error.Errors))
	for _, item := range completed.Error.Errors {
		if strings.TrimSpace(item.Message) != "" {
			messages = append(messages, item.Message)
			continue
		}
		if strings.TrimSpace(item.Code) != "" {
			messages = append(messages, item.Code)
		}
	}
	if len(messages) == 0 {
		return fmt.Errorf("%s gce instance op=%q failed", action, op.Name)
	}
	return fmt.Errorf("%s gce instance op=%q failed: %s", action, op.Name, strings.Join(messages, "; "))
}

func (r *GceRuntime) waitForTerminated(ctx context.Context, client gceComputeClient, instanceName string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		instance, found, err := r.getInstance(ctx, client, instanceName)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		status := strings.ToUpper(strings.TrimSpace(instance.Status))
		if status == "TERMINATED" || status == "SUSPENDED" {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *GceRuntime) instanceSpec(instanceName, cloudInit string) *gceInsertInstanceRequest {
	req := &gceInsertInstanceRequest{
		Name:        instanceName,
		MachineType: fmt.Sprintf("zones/%s/machineTypes/%s", r.zone, r.machineType),
	}
	req.Disks = []struct {
		AutoDelete       bool   `json:"autoDelete"`
		Boot             bool   `json:"boot"`
		Type             string `json:"type"`
		InitializeParams struct {
			SourceImage string `json:"sourceImage"`
			DiskSizeGb  int64  `json:"diskSizeGb"`
		} `json:"initializeParams"`
	}{
		{
			AutoDelete: true,
			Boot:       true,
			Type:       "PERSISTENT",
			InitializeParams: struct {
				SourceImage string `json:"sourceImage"`
				DiskSizeGb  int64  `json:"diskSizeGb"`
			}{
				SourceImage: fmt.Sprintf("projects/%s/global/images/family/%s", r.imageProject, r.imageFamily),
				DiskSizeGb:  r.diskSizeGB,
			},
		},
	}
	req.NetworkInterfaces = []struct {
		Network    string `json:"network"`
		Subnetwork string `json:"subnetwork"`
	}{
		{
			Network:    gceNetworkPath(r.project, r.network),
			Subnetwork: gceSubnetworkPath(r.project, r.zone, r.subnetwork),
		},
	}
	req.ServiceAccounts = []struct {
		Email  string   `json:"email"`
		Scopes []string `json:"scopes"`
	}{
		{
			Email:  r.serviceAccountEmail,
			Scopes: []string{"https://www.googleapis.com/auth/cloud-platform"},
		},
	}
	req.Metadata.Items = []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}{
		{Key: "user-data", Value: cloudInit},
	}
	return req
}

func (r *GceRuntime) buildArcadBinaryBase64(ctx context.Context) (string, error) {
	tmpDir, err := os.MkdirTemp("", "arca-gce-arcad-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	arcadPath := filepath.Join(tmpDir, "arcad")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", arcadPath, "./cmd/arcad")
	cmd.Env = append(os.Environ(),
		"GOOS="+r.arcadGOOS,
		"GOARCH="+r.arcadGOARCH,
		"CGO_ENABLED=0",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go build ./cmd/arcad failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	data, err := os.ReadFile(arcadPath)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func newGceRESTClient(ctx context.Context) (gceComputeClient, error) {
	httpClient, err := google.DefaultClient(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, err
	}
	return &gceRESTClient{httpClient: httpClient}, nil
}

func (c *gceRESTClient) GetInstance(ctx context.Context, project, zone, instance string) (*gceInstance, error) {
	var out gceInstance
	err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/compute/v1/projects/%s/zones/%s/instances/%s", url.PathEscape(project), url.PathEscape(zone), url.PathEscape(instance)), nil, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *gceRESTClient) InsertInstance(ctx context.Context, project, zone string, instance *gceInsertInstanceRequest) (*gceOperation, error) {
	var out gceOperation
	err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/compute/v1/projects/%s/zones/%s/instances", url.PathEscape(project), url.PathEscape(zone)), instance, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *gceRESTClient) StartInstance(ctx context.Context, project, zone, instance string) (*gceOperation, error) {
	var out gceOperation
	err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/compute/v1/projects/%s/zones/%s/instances/%s/start", url.PathEscape(project), url.PathEscape(zone), url.PathEscape(instance)), nil, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *gceRESTClient) StopInstance(ctx context.Context, project, zone, instance string) (*gceOperation, error) {
	var out gceOperation
	err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/compute/v1/projects/%s/zones/%s/instances/%s/stop", url.PathEscape(project), url.PathEscape(zone), url.PathEscape(instance)), nil, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *gceRESTClient) DeleteInstance(ctx context.Context, project, zone, instance string) (*gceOperation, error) {
	var out gceOperation
	err := c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/compute/v1/projects/%s/zones/%s/instances/%s", url.PathEscape(project), url.PathEscape(zone), url.PathEscape(instance)), nil, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *gceRESTClient) WaitZoneOperation(ctx context.Context, project, zone, operation string) (*gceOperation, error) {
	var out gceOperation
	err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/compute/v1/projects/%s/zones/%s/operations/%s/wait", url.PathEscape(project), url.PathEscape(zone), url.PathEscape(operation)), nil, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *gceRESTClient) doJSON(ctx context.Context, method, path string, body any, out any) error {
	url := "https://compute.googleapis.com" + path
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, payload)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		message := strings.TrimSpace(string(responseBody))
		var wrapped struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(responseBody, &wrapped); err == nil {
			if strings.TrimSpace(wrapped.Error.Message) != "" {
				message = strings.TrimSpace(wrapped.Error.Message)
			}
		}
		return &gceAPIError{StatusCode: resp.StatusCode, Message: message}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func isGceNotFound(err error) bool {
	if err == nil {
		return false
	}
	apiErr := &gceAPIError{}
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusNotFound
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "was not found")
}

func gceNetworkPath(project, network string) string {
	network = strings.TrimSpace(network)
	if strings.Contains(network, "/") {
		return network
	}
	return fmt.Sprintf("projects/%s/global/networks/%s", project, network)
}

func gceSubnetworkPath(project, zone, subnetwork string) string {
	subnetwork = strings.TrimSpace(subnetwork)
	if strings.Contains(subnetwork, "/") {
		return subnetwork
	}
	region := regionFromZone(zone)
	if region == "" {
		return subnetwork
	}
	return fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", project, region, subnetwork)
}

func regionFromZone(zone string) string {
	zone = strings.TrimSpace(zone)
	idx := strings.LastIndex(zone, "-")
	if idx <= 0 {
		return ""
	}
	return zone[:idx]
}

func gceInstanceIPv4(instance *gceInstance) string {
	if instance == nil {
		return ""
	}
	for _, nic := range instance.NetworkInterfaces {
		ip := strings.TrimSpace(nic.NetworkIP)
		if ip != "" {
			return ip
		}
	}
	return ""
}
