package talk

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
)

func unlockWebcam() {
	// v4l2-ctl --device /dev/video0 -c auto_exposure=3 -c white_balance_automatic=1
	cmd := exec.Command("v4l2-ctl", "--device", "/dev/video0", "-c", "auto_exposure=3", "-c", "white_balance_automatic=1")
	output, err := cmd.Output()

	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Command output: %s\n", output)
}

func lockWebcam(exposureTime, whiteBalanceTemperature int) {
	// v4l2-ctl --device /dev/video0 -c auto_exposure=1 -c exposure_time_absolute=305 -c white_balance_automatic=0 -c white_balance_temperature=8000
	cmd := exec.Command("v4l2-ctl", "--device", "/dev/video0",
		"-c", "auto_exposure=1",
		"-c", fmt.Sprintf("exposure_time_absolute=%d", exposureTime),
		"-c", "white_balance_automatic=0",
		"-c", fmt.Sprintf("white_balance_temperature=%d", whiteBalanceTemperature))
	output, err := cmd.Output()

	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Command output: %s\n", output)
}

func getWebcamExposureTime() int {
	cmd := exec.Command("v4l2-ctl", "--device", "/dev/video0", "-C", "exposure_time_absolute")
	output, err := cmd.Output()

	if err != nil {
		log.Fatal(err)
		return -1
	}

	parts := strings.Split(string(output), ":")
	exposureTime, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		fmt.Println("Error:", err)
		return -1
	}

	return exposureTime
}

func getWebcamwhiteBalanceTemperature() int {
	cmd := exec.Command("v4l2-ctl", "--device", "/dev/video0", "-C", "white_balance_temperature")
	output, err := cmd.Output()

	if err != nil {
		log.Fatal(err)
		return -1
	}

	parts := strings.Split(string(output), ":")
	whiteBalanceTemperature, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		fmt.Println("Error:", err)
		return -1
	}

	return whiteBalanceTemperature
}
