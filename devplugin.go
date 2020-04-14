package sgx

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"syscall"
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1beta1"
)

const (
	resourceName   = "microsoft.io/cc_enabled"
	pluginEndpoint = "sgxlkl.sock"
	socketPath     = pluginapi.DevicePluginPath + pluginEndpoint
	devicePath     = "/opt/sgxlkl"
)

// SGXLKLManager implements device plugin interface
type SGXLKLManager struct {
	devices map[string]*pluginapi.Device
	stop    chan interface{}
	server  *grpc.Server
}

// NewSGXLKLManager creates SGXLKLManager
func NewSGXLKLManager() (*SGXLKLManager, error) {
	m := &SGXLKLManager{
		stop: make(chan interface{}),
	}

	if err := m.checkSGXLKLPresent(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *SGXLKLManager) checkSGXLKLPresent() error {
	// confirm presense of SGX-LKL driver
	stat := syscall.Stat_t{}
	if err = syscall.Stat(devicePath, &stat); err != nil {
		return err
	}
	// allocate devices
	m.devices = make(map[string]*pluginapi.Device)
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("%03d", i)
		m.devices[id] = &pluginapi.Device{ID: id, Health: pluginapi.Healthy}
	}
	return nil
}

// Run starts gRPC server and register SGX device plugin to Kubelet
func (m *SGXLKLManager) Run() error {
	err := m.Start()
	if err != nil {
		glog.Errorf("Could not start device plugin: %v", err)
		return err
	}
	glog.Infof("SGX-LKL device plugin socket path: %s", socketPath)

	err = m.Register()
	if err != nil {
		glog.Errorf("Could not register SGX-LKL device plugin: %v", err)
		m.Stop()
		return err
	}
	glog.Infof("SGX device plugin is running")

	return nil
}

// Start starts gRPC server
func (m *SGXLKLManager) Start() error {
	glog.Infof("SGX-LKL device plugin: Start")
	err := m.cleanup()
	if err != nil {
		return err
	}
	sock, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	m.server = grpc.NewServer([]grpc.ServerOption{}...)
	pluginapi.RegisterDevicePluginServer(m.server, m)
	go m.server.Serve(sock)

	return nil
}

// Stop stops gRPC server
func (m *SGXLKLManager) Stop() error {
	glog.Infof("SGX-LKL device plugin: Stop")
	if m.server == nil {
		return nil
	}
	m.server.Stop()
	m.server = nil
	close(m.stop)
	return m.cleanup()
}

// Register registers SGX-LKL device plugin with kubelet
func (m *SGXLKLManager) Register() error {
	glog.Infof("SGX-LKL device plugin: Register")

	conn, err := grpc.Dial(pluginapi.KubeletSocket, grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pluginapi.NewRegistrationClient(conn)

	req := &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     path.Base(socketPath),
		ResourceName: resourceName,
	}
	_, err = client.Register(context.Background(), req)
	if err != nil {
		return err
	}
	return nil
}

// ListAndWatch implements DevicePlugin API call
func (m *SGXLKLManager) ListAndWatch(emtpy *pluginapi.Empty, stream pluginapi.DevicePlugin_ListAndWatchServer) error {
	glog.Infof("SGX-LKL device plugin: ListAndWatch")
	resp := new(pluginapi.ListAndWatchResponse)
	for _, dev := range m.devices {
		resp.Devices = append(resp.Devices, dev)
	}
	if err := stream.Send(resp); err != nil {
		glog.Infof("ListAndWatch failed to send responce to kubelet: %v", err)
		return err
	}
	for {
		select {
		case <-m.stop:
			glog.Infof("SGX-LKL device plugin: ListAndWatch exit")
			return nil
		}
	}
}

// Allocate implements DevicePlugin API call
func (m *SGXLKLManager) Allocate(ctx context.Context, req *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	glog.Infof("SGX-LKL device plugin: Allocate")
	resp := new(pluginapi.AllocateResponse)
	for _, creq := range req.ContainerRequests {
		cresp := new(pluginapi.ContainerAllocateResponse)
		glog.Infof("Request devices %v", creq.DevicesIDs)
		cresp.Devices = append(cresp.Devices, &pluginapi.DeviceSpec{
			HostPath:      devicePath,
			ContainerPath: devicePath,
			Permissions:   "rw",
		})
		resp.ContainerResponses = append(resp.ContainerResponses, cresp)
	}
	return resp, nil
}

// GetDevicePluginOptions implements DevicePlugin API call
func (m *SGXLKLManager) GetDevicePluginOptions(context.Context, *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

// PreStartContainer implements DevicePlugin API call
func (m *SGXLKLManager) PreStartContainer(context.Context, *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

func (m *SGXLKLManager) cleanup() error {
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
