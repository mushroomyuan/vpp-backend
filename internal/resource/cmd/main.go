package main

import (
	resource "github.com/mushroomyuan/vpp-backend/resource"
)

func main() {
	resource.NewApp("vpp-resource").Run()
}
