package main

import dispatch "github.com/mushroomyuan/vpp-backend/dispatch"

func main() {
	dispatch.NewApp("vpp-dispatch").Run()
}
