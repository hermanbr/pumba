package docker

import (
	"context"
	"fmt"
	"strings"

	ctr "github.com/alexei-led/pumba/pkg/container"
	log "github.com/sirupsen/logrus"
)

// IPTablesContainer injects sidecar iptables container into the given container network namespace
func (client dockerClient) IPTablesContainer(ctx context.Context, req *ctr.IPTablesRequest) error {
	log.WithFields(log.Fields{
		"name":          req.Container.Name(),
		"id":            req.Container.ID(),
		"commandPrefix": req.CmdPrefixes,
		"commandSuffix": req.CmdSuffix,
		"srcIPs":        req.SrcIPs,
		"dstIPs":        req.DstIPs,
		"sports":        req.SPorts,
		"dports":        req.DPorts,
		"duration":      req.Duration,
		"img":           req.Sidecar.Image,
		"pull":          req.Sidecar.Pull,
		"dryrun":        req.DryRun,
	}).Info("running iptables on container")
	if len(req.SrcIPs) == 0 && len(req.DstIPs) == 0 && len(req.SPorts) == 0 && len(req.DPorts) == 0 {
		return client.ipTablesContainer(ctx, req)
	}
	return client.ipTablesContainerWithIPFilter(ctx, req)
}

// StopIPTablesContainer stops the iptables container injected into the given container network namespace
func (client dockerClient) StopIPTablesContainer(ctx context.Context, req *ctr.IPTablesRequest) error {
	log.WithFields(log.Fields{
		"name":          req.Container.Name(),
		"id":            req.Container.ID(),
		"commandPrefix": req.CmdPrefixes,
		"commandSuffix": req.CmdSuffix,
		"srcIPs":        req.SrcIPs,
		"dstIPs":        req.DstIPs,
		"sports":        req.SPorts,
		"dports":        req.DPorts,
		"img":           req.Sidecar.Image,
		"pull":          req.Sidecar.Pull,
		"dryrun":        req.DryRun,
	}).Info("stopping iptables on container")
	if len(req.SrcIPs) == 0 && len(req.DstIPs) == 0 && len(req.SPorts) == 0 && len(req.DPorts) == 0 {
		return client.ipTablesContainer(ctx, req)
	}
	return client.ipTablesContainerWithIPFilter(ctx, req)
}

func (client dockerClient) ipTablesContainer(ctx context.Context, req *ctr.IPTablesRequest) error {
	log.WithFields(log.Fields{
		"name":        req.Container.Name(),
		"id":          req.Container.ID(),
		"cmdPrefixes": req.CmdPrefixes,
		"cmdSuffix":   strings.Join(req.CmdSuffix, " "),
		"img":         req.Sidecar.Image,
		"pull":        req.Sidecar.Pull,
		"dryrun":      req.DryRun,
	}).Debug("execute iptables for container")
	if !req.DryRun {
		var commands [][]string
		for _, prefix := range req.CmdPrefixes {
			command := append([]string{}, prefix...)
			command = append(command, req.CmdSuffix...)
			log.WithField("iptables", strings.Join(command, " ")).Debug("executing iptables")
			commands = append(commands, command)
		}
		return client.ipTablesCommands(ctx, req.Container, commands, req.Sidecar.Image, req.Sidecar.Pull)
	}
	return nil
}

func (client dockerClient) ipTablesContainerWithIPFilter(ctx context.Context, req *ctr.IPTablesRequest) error {
	log.WithFields(log.Fields{
		"name":   req.Container.Name(),
		"id":     req.Container.ID(),
		"srcIPs": req.SrcIPs,
		"dstIPs": req.DstIPs,
		"Sports": req.SPorts,
		"Dports": req.DPorts,
		"img":    req.Sidecar.Image,
		"pull":   req.Sidecar.Pull,
		"dryrun": req.DryRun,
	}).Debug("execute iptables for container with IP(s) filter")
	if !req.DryRun {
		// use docker client ExecStart to run iptables rules to filter network.
		// One rule per (chain prefix × IP/port filter): with --bidirectional the
		// same filters are replicated onto the OUTPUT prefix as well as INPUT.
		// See more about the iptables statistics extension: https://www.man7.org/linux/man-pages/man8/iptables-extensions.8.html
		var commands [][]string
		for _, prefix := range req.CmdPrefixes {
			commands = append(commands, ipTablesFilterCommands(prefix, req)...)
		}

		err := client.ipTablesCommands(ctx, req.Container, commands, req.Sidecar.Image, req.Sidecar.Pull)
		if err != nil {
			return fmt.Errorf("failed to run iptables commands: %w", err)
		}
	}
	return nil
}

// ipTablesFilterCommands builds one iptables command per IP/port filter for a
// single chain prefix (matching the per-filter rule expansion), each combining
// prefix + the specific match + suffix.
func ipTablesFilterCommands(prefix []string, req *ctr.IPTablesRequest) [][]string {
	var commands [][]string
	appendCmd := func(matchFlag, matchValue string) {
		cmd := append([]string{}, prefix...)
		cmd = append(cmd, matchFlag, matchValue)
		cmd = append(cmd, req.CmdSuffix...)
		commands = append(commands, cmd)
	}
	for _, ip := range req.SrcIPs { // # drop traffic to a specific source address
		appendCmd("-s", ip.String())
	}
	for _, ip := range req.DstIPs { // # drop traffic to a specific destination address
		appendCmd("-d", ip.String())
	}
	for _, sport := range req.SPorts { // # drop traffic to a specific source port
		appendCmd("--sport", sport)
	}
	for _, dport := range req.DPorts { // # drop traffic to a specific destination port
		appendCmd("--dport", dport)
	}
	return commands
}

func (client dockerClient) ipTablesCommands(ctx context.Context, c *ctr.Container, argsList [][]string, tcimg string, pull bool) error {
	if tcimg == "" {
		for _, args := range argsList {
			if err := client.execOnContainer(ctx, c, "iptables", args, true); err != nil {
				return fmt.Errorf("error running iptables command on container: %v: %w", strings.Join(args, " "), err)
			}
		}
		return nil
	}
	return client.runSidecar(ctx, c, argsList, tcimg, "iptables", pull)
}
