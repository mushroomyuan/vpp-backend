package main

import gateway "github.com/mushroomyuan/vpp-backend/gateway"

func main() {
	gateway.NewApp("vpp-gateway").Run()
}
