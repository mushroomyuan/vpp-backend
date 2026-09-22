package main

import forecast "github.com/mushroomyuan/vpp-backend/forecast"

func main() {
	forecast.NewApp("vpp-forecast").Run()
}
