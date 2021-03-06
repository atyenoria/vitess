// Copyright 2015, Google Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testlib

import (
	"fmt"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/youtube/vitess/go/vt/topo"
	"github.com/youtube/vitess/go/vt/vtctl/grpcvtctlserver"
	"github.com/youtube/vitess/go/vt/vtctl/vtctlclient"
	"golang.org/x/net/context"

	// we need to import the grpcvtctlclient library so the gRPC
	// vtctl client is registered and can be used.
	_ "github.com/youtube/vitess/go/vt/vtctl/grpcvtctlclient"
)

func init() {
	// make sure we use the right protocol
	*vtctlclient.VtctlClientProtocol = "grpc"
}

// VtctlPipe is a vtctl server based on a topo server, and a client that
// is connected to it via bson rpc.
type VtctlPipe struct {
	listener net.Listener
	client   vtctlclient.VtctlClient
	t        *testing.T
}

// NewVtctlPipe creates a new VtctlPipe based on the given topo server.
func NewVtctlPipe(t *testing.T, ts topo.Server) *VtctlPipe {
	// Listen on a random port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Cannot listen: %v", err)
	}

	// Create a gRPC server and listen on the port
	server := grpc.NewServer()
	grpcvtctlserver.StartServer(server, ts)
	go server.Serve(listener)

	// Create a VtctlClient gRPC client to talk to the fake server
	client, err := vtctlclient.New(listener.Addr().String(), 30*time.Second)
	if err != nil {
		t.Fatalf("Cannot create client: %v", err)
	}

	return &VtctlPipe{
		listener: listener,
		client:   client,
		t:        t,
	}
}

// Close will stop listening and free up all resources.
func (vp *VtctlPipe) Close() {
	vp.client.Close()
	vp.listener.Close()
}

// Run executes the provided command remotely, logs the output in the
// test logs, and returns the command error.
func (vp *VtctlPipe) Run(args []string) error {
	actionTimeout := 30 * time.Second
	lockTimeout := 10 * time.Second
	ctx := context.Background()

	c, errFunc, err := vp.client.ExecuteVtctlCommand(ctx, args, actionTimeout, lockTimeout)
	if err != nil {
		return fmt.Errorf("VtctlPipe.Run() failed: %v", err)
	}
	for le := range c {
		vp.t.Logf(le.String())
	}
	return errFunc()
}
