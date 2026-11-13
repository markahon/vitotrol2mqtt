package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/benvanmierloo/go-vitotrol"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var pConf *Config
var pVitotrol *vitotrol.Session
var mqttClient mqtt.Client

var customAttrRegEx = regexp.MustCompile(
	`^([a-zA-Z0-9_]+)[-_]0x([a-fA-F0-9]{1,4})\z`)

// updateDeviceAttr writes one attribute. Errors are logged, not fatal: a failed
// write must not stop polling.
func updateDeviceAttr(deviceName string, attrName string, value string) {
	if err := writeAttr(deviceName, attrName, value); err != nil {
		fmt.Fprintf(os.Stderr, "WriteData %s: %s\n", attrName, err)
		fmt.Printf("Device %s: cannot set attribute %s to %s\n", deviceName, attrName, value)
	}
}

// writeAttr writes one read-write attribute and publishes the new value.
func writeAttr(deviceName string, attrName string, value string) error {
	writeMutex.Lock()
	defer writeMutex.Unlock()

	attrId, ok := vitotrol.AttributesNames2IDs[attrName]
	if !ok || vitotrol.AttributesRef[attrId].Access != vitotrol.ReadWrite {
		return errors.New("not a read-write attribute")
	}
	fmt.Printf("Setting %s to %s\n", attrName, value)
	// the server wants doubles like "21,5"
	vitodataValue, err := vitotrol.AttributesRef[attrId].Type.Human2VitodataValue(value)
	if err != nil {
		return fmt.Errorf("bad value: %w", err)
	}
	for _, vdev := range pVitotrol.Devices {
		if vdev.DeviceName == deviceName {
			ch, err := vdev.WriteDataWait(pVitotrol, attrId, vitodataValue)
			if err != nil {
				return err
			}
			if err = <-ch; err != nil {
				return err
			}
			// update MQTT with the new value
			mqttClient.Publish(pConf.MQTT.Topic+"/"+vdev.DeviceName+"/"+attrName, 0, false, value).Wait()
			return nil
		}
	}
	return errors.New("unknown device")
}

// writeMutex serialises writes to the controller.
var writeMutex sync.Mutex

// publishSchedule reads timesheet id and publishes it retained as JSON
// ({"mon":[{"from":630,"to":900,"value":2}],...}) on
// <topic>/<device>/Schedule_<id>.
func publishSchedule(vdev *vitotrol.Device, id vitotrol.TimesheetID) bool {
	if err := vdev.GetTimesheetData(pVitotrol, id); err != nil {
		fmt.Fprintf(os.Stderr, "GetTimesheetData %d error: %s\n", id, err)
		return false
	}
	b, _ := json.Marshal(vdev.Timesheets[id])
	token := mqttClient.Publish(fmt.Sprintf("%s/%s/Schedule_%d", pConf.MQTT.Topic, vdev.DeviceName, id), 0, true, b)
	token.Wait()
	return true
}

// updateDeviceSchedule handles <topic>/<device>/Schedule_<id>/get (publish
// only) and /set (JSON like {"mon-sun":[{"from":630,"to":900,"value":2}]},
// {"mon-sun":[]} = no slots), then publishes the read-back. Errors are
// logged, not fatal: a bad payload must not crash-loop the bridge.
func updateDeviceSchedule(deviceName string, id vitotrol.TimesheetID, cmd string, payload []byte) {
	writeMutex.Lock()
	defer writeMutex.Unlock()

	var vdev *vitotrol.Device
	for i := range pVitotrol.Devices {
		if pVitotrol.Devices[i].DeviceName == deviceName {
			vdev = &pVitotrol.Devices[i]
		}
	}
	if vdev == nil {
		fmt.Printf("Device %s: unknown, Schedule_%d/%s ignored\n", deviceName, id, cmd)
		return
	}
	// read first: the write must echo the timesheet type learnt here
	if !publishSchedule(vdev, id) || cmd != "set" {
		return
	}
	var data map[string]vitotrol.TimeslotSlice
	if err := json.Unmarshal(payload, &data); err != nil {
		fmt.Fprintf(os.Stderr, "Schedule_%d: bad JSON: %s\n", id, err)
		return
	}
	fmt.Printf("Setting Schedule_%d to %s\n", id, payload)
	ch, err := vdev.WriteTimesheetDataWait(pVitotrol, id, data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WriteTimesheetData %d error: %s\n", id, err)
	} else if err = <-ch; err != nil {
		fmt.Fprintf(os.Stderr, "WriteTimesheetData %d failed: %s\n", id, err)
	}
	publishSchedule(vdev, id)
}

var messagePubHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	if pConf != nil {
		// Own goroutines: a write waits up to minutes for the controller,
		// longer than the MQTT keepalive this callback would block.
		var scheduleRegEx = regexp.MustCompile(pConf.MQTT.Topic + `\/(.*?)\/Schedule_(\d+)\/(set|get)$`)
		if m := scheduleRegEx.FindStringSubmatch(msg.Topic()); m != nil {
			id, _ := strconv.ParseUint(m[2], 10, 16)
			go updateDeviceSchedule(m[1], vitotrol.TimesheetID(id), m[3], msg.Payload())
			return
		}

		var topicRegEx = regexp.MustCompile(pConf.MQTT.Topic + `\/(.*?)\/([^/]*?)\/set`)
		m := topicRegEx.FindStringSubmatch(msg.Topic())

		if m != nil {
			go updateDeviceAttr(m[1], m[2], string(msg.Payload()))
		}
	}
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	fmt.Println("MQTT Connected")
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	fmt.Printf("MQTT Connect lost: %v", err)
	os.Exit(1)
}

func VitotrolInit(vconf *ConfigVitotrol) *vitotrol.Session {
	pVitotrol = &vitotrol.Session{
		Debug: vconf.Debug,
	}

	fmt.Println("Vitotrol login...")
	err := pVitotrol.Login(vconf.Login, vconf.Password)
	if err != nil {
		fmt.Println("Login failed:", err)
		// don't retry a rejected login every minute, it can lock the account
		time.Sleep(time.Duration(vconf.RetryTimeout) * time.Second)
		os.Exit(1)
	}

	fmt.Println("Vitotrol GetDevices...")
	err = pVitotrol.GetDevices()
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetDevices failed: %s\n", err)
		os.Exit(1)
	}
	if len(pVitotrol.Devices) == 0 {
		fmt.Fprintln(os.Stderr, "no device found")
		os.Exit(1)
	}
	fmt.Printf("%d device(s) found\n", len(pVitotrol.Devices))
	return pVitotrol
}

// getAttrValue returns the value of attrID, nil if the server did not send it.
// A value its type can't parse is returned as is: custom attributes are all
// Double, even dates.
func getAttrValue(vdev *vitotrol.Device, attrID vitotrol.AttrID) (value interface{}) {
	pValue := vdev.Attributes[attrID]
	if pValue == nil {
		return nil
	}
	value, err := vitotrol.AttributesRef[attrID].
		Type.Vitodata2NativeValue(pValue.Value)
	if err != nil {
		return pValue.Value
	}

	// uint64 handled from influx 1.4
	if vuint64, ok := value.(uint64); ok {
		value = int(vuint64)
	}
	return
}

// A refresh timeout means the gateway lost its cloud link. Short losses (a few
// minutes, several a day, logs 2026-11) are ridden out in-process: exiting on
// each one flapped every HA entity. Long ones used to need a manual nudge (change
// any value on the Viessmann portal), so after wakeupAfter the bridge rewrites
// wakeupAttr with its last read value. After giveUpAfter it exits (crash-only:
// fresh login, which reports a disconnected device).
const (
	wakeupAfter = 15 * time.Minute
	giveUpAfter = 45 * time.Minute
	wakeupAttr  = "RWNormalRoomTemp"
)

var (
	failingSince time.Time // zero = last refresh worked
	timeouts     int
	wakeupSent   bool
	wakeupValue  string // last read value of wakeupAttr
	lastState    string
)

// timeoutAction decides what to do after a refresh timeout that started
// elapsed ago: "skip" (retry next cycle), "wakeup" (nudge once, then retry) or
// "exit".
func timeoutAction(elapsed time.Duration, wakeupSent bool) string {
	switch {
	case elapsed >= giveUpAfter:
		return "exit"
	case elapsed >= wakeupAfter && !wakeupSent:
		return "wakeup"
	}
	return "skip"
}

// publishStatus publishes the bridge state retained on <topic>/bridge/status:
// ok | timeouts | wakeup_sent | wakeup_failed. HA logs it and warns on wakeup_failed.
func publishStatus(state, detail string) {
	lastState = state
	since := ""
	if !failingSince.IsZero() {
		since = failingSince.Format(time.RFC3339)
	}
	b, _ := json.Marshal(map[string]interface{}{
		"state": state, "timeouts": timeouts, "since": since, "detail": detail,
		"wakeup_sent": wakeupSent, "time": time.Now().Format(time.RFC3339),
	})
	fmt.Printf("Status: %s\n", b)
	mqttClient.Publish(pConf.MQTT.Topic+"/bridge/status", 0, true, b).Wait()
}

// onRefreshTimeout handles one refresh timeout; it returns only if the bridge
// should keep running.
func onRefreshTimeout(deviceName string) {
	if failingSince.IsZero() {
		failingSince = time.Now()
	}
	timeouts++
	switch timeoutAction(time.Since(failingSince), wakeupSent) {
	case "exit":
		state := "timeouts"
		if wakeupSent {
			state = "wakeup_failed"
		}
		publishStatus(state, "giving up, restarting")
		os.Exit(1)
	case "wakeup":
		wakeupSent = true
		result := "no value read yet, nothing written"
		if wakeupValue != "" {
			if err := writeAttr(deviceName, wakeupAttr, wakeupValue); err != nil {
				result = fmt.Sprintf("rewrote %s=%s: %s", wakeupAttr, wakeupValue, err)
			} else {
				result = fmt.Sprintf("rewrote %s=%s: accepted", wakeupAttr, wakeupValue)
			}
		}
		publishStatus("wakeup_sent", result)
	default:
		if lastState != "wakeup_sent" {
			publishStatus("timeouts", "")
		}
	}
}

// onRefreshOK publishes the recovery (or the first poll after start).
func onRefreshOK() {
	if lastState == "ok" {
		return
	}
	detail := "started"
	if !failingSince.IsZero() {
		detail = fmt.Sprintf("recovered after %d timeouts, %s", timeouts, time.Since(failingSince).Round(time.Second))
		if wakeupSent {
			detail += ", after wake-up write"
		}
	}
	failingSince, timeouts, wakeupSent = time.Time{}, 0, false
	publishStatus("ok", detail)
}

func refreshDevice(device *vitotrol.Device, attrs []vitotrol.AttrID) bool {
	ch, err := device.RefreshDataWait(pVitotrol, attrs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "RefreshData error: %s\n", err)
		os.Exit(1)
	}
	if err = <-ch; err != nil {
		fmt.Fprintf(os.Stderr, "RefreshData failed: %s\n", err)
		if !errors.Is(err, vitotrol.ErrTimeout) {
			os.Exit(1)
		}
		onRefreshTimeout(device.DeviceName)
		return false // keep the session and the last values, retry next cycle
	}

	err = device.GetData(pVitotrol, attrs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetData error: %s\n", err)
		os.Exit(1)
	}

	fields := map[string]interface{}{}

	for _, attrID := range attrs {
		if value := getAttrValue(device, attrID); value != nil {
			fields[vitotrol.AttributesRef[attrID].Name] = value
		}
	}

	if v, ok := fields[wakeupAttr]; ok {
		wakeupValue = fmt.Sprint(v)
	}
	onRefreshOK()

	// Write the batch
	for key, element := range fields {
		token := mqttClient.Publish(pConf.MQTT.Topic+"/"+device.DeviceName+"/"+key, 0, false, fmt.Sprint(element))
		token.Wait()
	}

	return true
}

func refreshDevices() {
	for {
		start := time.Now()
		for _, device := range pVitotrol.Devices {
			// Check if this device has a configuration
			deviceConfig := pConf.GetConfigDevice(device.DeviceName, device.LocationName)
			if deviceConfig != nil {
				if device.IsConnected {
					refreshDevice(&device, deviceConfig.attrs)
				} else {
					// Device is not connect - retry.
					fmt.Fprintf(os.Stderr, "Device is not connected `%s'\n", device.DeviceName)
					// ponytail: crash-only kept; wait retry_timeout before exiting so a gateway
					// outage doesn't re-login every minute. Keep it >= minutes: a run >10s resets
					// Docker's restart backoff. Real fix = local Optolink.
					time.Sleep(time.Duration(pConf.Vitotrol.RetryTimeout) * time.Second)
					os.Exit(1)
				}
			}
		}
		delta := time.Duration(pConf.Vitotrol.Frequency)*time.Second - time.Since(start)
		if delta > 0 {
			time.Sleep(delta)
		}
	}
}

func resolveFields() {
	for _, configDevice := range pConf.Devices {
		attrs := make(map[vitotrol.AttrID]struct{}, len(configDevice.Fields))

		for _, fieldName := range configDevice.Fields {
			// Already known attribute
			attrID, ok := vitotrol.AttributesNames2IDs[fieldName]
			if !ok {
				// Custom attribute

				m := customAttrRegEx.FindStringSubmatch(fieldName)
				if m == nil {
					fmt.Fprintf(os.Stderr, "Unknown attribute `%s'\n", fieldName)
					os.Exit(1)
				}

				tmpName := m[1]
				access := vitotrol.ReadOnly

				// TODO Better would be to do configDevice.GetTypeInfo and cache the result
				// and check access against the real type mapping.
				if strings.HasPrefix(tmpName, "RW") {
					access = vitotrol.ReadWrite
				}

				attrRef := vitotrol.AttrRef{
					Type:   vitotrol.TypeDouble,
					Access: access,
					Name:   tmpName,
				}
				tmpID, _ := strconv.ParseUint(m[2], 16, 16)
				attrID = vitotrol.AttrID(tmpID)

				vitotrol.AddAttributeRef(attrID, attrRef)
			}

			attrs[attrID] = struct{}{}
		}

		if len(attrs) == 0 {
			fmt.Fprintf(os.Stderr, "No attributes for device %s/location %s\n",
				configDevice.Name, configDevice.Location)
			os.Exit(1)
		}

		configDevice.attrs = make([]vitotrol.AttrID, 0, len(attrs))
		for attrID := range attrs {
			configDevice.attrs = append(configDevice.attrs, attrID)
		}
	}
}

func initializeMQTTClient() {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://" + pConf.MQTT.Broker + ":" + pConf.MQTT.Port)
	opts.SetClientID(pConf.MQTT.ClientID)
	opts.SetUsername(pConf.MQTT.Login)
	opts.SetPassword(pConf.MQTT.Password)
	opts.SetAutoReconnect(false)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectLostHandler
	mqttClient = mqtt.NewClient(opts)

	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	//subscribe to the topic to catch the control commands
	topic := pConf.MQTT.Topic + "/#"
	if token := mqttClient.Subscribe(topic, 1, messagePubHandler); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
}

func mainLoop() {
	pVitotrol = VitotrolInit(&pConf.Vitotrol)

	for {
		refreshDevices()
	}
}

func main() {
	//read configuration
	configFile := flag.String("config", "", "config file")

	flag.Parse()

	if *configFile == "" {
		fmt.Fprintln(os.Stderr, "config file is missing")
		os.Exit(1)
	}
	conf, err := ReadConfig(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot read config: %s\n", err)
		os.Exit(1)
	}
	pConf = conf

	// Timesheet writes take ~1 min to reach the controller: poll every 5 s
	// (default 1 s), give up after 3 min.
	vitotrol.WriteTimesheetDataWaitMinDuration = 5 * time.Second
	vitotrol.WriteTimesheetDataWaitTimeout = 3 * time.Minute

	// Resolve fields
	resolveFields()

	// Create a new MQTT Client
	initializeMQTTClient()

	mainLoop()
}
