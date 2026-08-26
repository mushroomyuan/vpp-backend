package main

import alarm "github.com/mushroomyuan/vpp-backend/alarm"

func main() {
	alarm.NewApp("vpp-alarm").Run()
}
