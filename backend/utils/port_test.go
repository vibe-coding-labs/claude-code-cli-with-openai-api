package utils

import (
	"fmt"
	"math/rand"
	"net"
	"testing"
)

func TestIsPortAvailable(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to bind test listener: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port

	if IsPortAvailable(port) {
		t.Errorf("expected port %d to be reported unavailable while held", port)
	}

	l.Close()

	if !IsPortAvailable(port) {
		t.Errorf("expected port %d to be reported available after release", port)
	}
}

func TestFindAvailablePort_SkipsOccupiedPort(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to bind test listener: %v", err)
	}
	defer l.Close()
	occupied := l.Addr().(*net.TCPAddr).Port

	got, err := FindAvailablePort(occupied)
	if err != nil {
		t.Fatalf("FindAvailablePort() error = %v", err)
	}
	if got == occupied {
		t.Errorf("expected FindAvailablePort to skip the occupied port %d", occupied)
	}
	if got < occupied || got >= occupied+100 {
		t.Errorf("returned port %d outside expected scan range [%d, %d)", got, occupied, occupied+100)
	}
}

// bindConsecutivePorts 尝试绑定 [base, base+count) 范围内的全部端口；
// 一旦某个端口已被占用（例如被其它进程抢先使用），立即释放已绑定的端口并返回失败，
// 供调用方换一个随机基准端口重试。
func bindConsecutivePorts(base, count int) ([]net.Listener, bool) {
	listeners := make([]net.Listener, 0, count)
	for i := 0; i < count; i++ {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", base+i))
		if err != nil {
			for _, ln := range listeners {
				ln.Close()
			}
			return nil, false
		}
		listeners = append(listeners, l)
	}
	return listeners, true
}

func TestFindAvailablePort_ExhaustsRangeReturnsError(t *testing.T) {
	var listeners []net.Listener
	var base int
	ok := false
	for attempt := 0; attempt < 8 && !ok; attempt++ {
		base = 20000 + rand.Intn(30000)
		listeners, ok = bindConsecutivePorts(base, 100)
	}
	if !ok {
		t.Skip("无法在本机找到 100 个连续空闲端口来模拟端口耗尽场景，环境端口占用过多，跳过")
	}
	defer func() {
		for _, l := range listeners {
			l.Close()
		}
	}()

	_, err := FindAvailablePort(base)
	if err == nil {
		t.Fatalf("expected error when the entire 100-port scan range is occupied")
	}
}
