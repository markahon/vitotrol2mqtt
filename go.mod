module github.com/markahon/vitotrol2mqtt

go 1.18

replace github.com/benvanmierloo/go-vitotrol => github.com/markahon/go-vitotrol v0.0.0-20261106160242-3f3e79f1842f

require (
	github.com/benvanmierloo/go-vitotrol v0.0.0-20261103233059-fa4534a960ac
	github.com/eclipse/paho.mqtt.golang v1.3.1
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/gorilla/websocket v1.4.2 // indirect
	golang.org/x/net v0.0.0-20200425230154-ff2c4b7c35a0 // indirect
)
