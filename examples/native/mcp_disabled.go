//go:build !mygo_mcp

package main

func startAutomation() func() { return func() {} }
