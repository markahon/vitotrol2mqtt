module github.com/markahon/vitotrol2mqtt

go 1.18

// replace github.com/maxatome/go-vitotrol => ../go-vitotrol

require (
	github.com/eclipse/paho.mqtt.golang v1.3.1
	github.com/maxatome/go-vitotrol v0.0.0-20210219214040-83ee003feff7
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/gorilla/websocket v1.4.2 // indirect
	golang.org/x/net v0.0.0-20200425230154-ff2c4b7c35a0 // indirect
)
