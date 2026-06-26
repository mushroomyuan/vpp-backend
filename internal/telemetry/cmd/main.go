package main

import (
	telemetry "github.com/mushroomyuan/vpp-backend/telemetry"
)

func main() {
	telemetry.NewApp("vpp-telemetry").Run()
}
