package bootstrap

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

const Version = "0.1.0"

func DataDir() string {
	if v := os.Getenv("MACE_HOME"); v != "" {
		return v
	}
	home, e := os.UserConfigDir()
	if e != nil {
		return "./data"
	}
	return filepath.Join(home, "mace")
}

func Token(path string) (string, error) {
	if v := os.Getenv("MACE_AUTH_SECRET"); v != "" {
		return v, nil
	}
	if b, e := os.ReadFile(path); e == nil && len(b) >= 32 {
		return string(b), nil
	}
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return "", e
	}
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	value := hex.EncodeToString(b)
	if e := os.WriteFile(path, []byte(value), 0600); e != nil {
		return "", e
	}
	return value, nil
}
func Brand() {
	fmt.Print("\n  __  __    _    ____ _____\n |  \\/  |  / \\  / ___| ____|\n | |\\/| | / _ \\| |   |  _|\n | |  | |/ ___ \\ |___| |___\n |_|  |_/_/   \\_\\____|_____|\n\n")
	fmt.Printf("MACE %s · Multi Agent Coordination and Execution\n\n", Version)
}
func Banner(token, host string, port int, log string) {
	Brand()
	if os.Getenv("MACE_TS_AUTHKEY") != "" {
		name := os.Getenv("MACE_TS_HOSTNAME")
		if name == "" {
			name = "mace-node"
		}
		fmt.Printf("Tailnet address: http://%s:%d/#connect=%s\n", name, port, token)
	}
	local := "127.0.0.1"
	if host != "" && host != "0.0.0.0" && host != "::" {
		// A server bound to one address answers only there.
		local = host
	}
	fmt.Printf("Open this computer: http://%s/#connect=%s\n", net.JoinHostPort(local, strconv.Itoa(port)), token)
	if host == "0.0.0.0" || host == "" {
		if addresses, e := net.InterfaceAddrs(); e == nil {
			for _, address := range addresses {
				if ip, ok := address.(*net.IPNet); ok && !ip.IP.IsLoopback() && ip.IP.To4() != nil {
					fmt.Printf("Open another device: http://%s:%d/#connect=%s\n", ip.IP, port, token)
				}
			}
		}
	}
	fmt.Printf("\nAccess token: %s\nLog: %s\n\nChoose an agent in Chats to start.\nmace status · mace stop · mace kill\n\n", token, log)
}
