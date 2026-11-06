
# Viessmann™ Vitotrol™ device to MQTT

Typically used to send Viessmann™ Vitotrol™ boiler data to a MQTT broker

## Installation


```sh
go build -v
```

will generate a `vitotrol2mqtt` executable.


## Usage

Create a YAML file based on
[`vitotrol2mqtt.yml`](vitotrol2mqtt.yml), including your
credentials and the attributes you want to send to the MQTT broker.

Registered attributes can be found here:
https://github.com/maxatome/go-vitotrol/blob/master/attributes.go#L79
(field `Name`).

Note! Including field mappings that are unsupported by your Viessmann device
will cause execution to crash and exit.

### Custom attributes

If you want an attribute that is not registered, use a name like
`NAME_0xNNNN` where `NAME` is the name of the attribute, and `NNNN` is
the hexadecimal representation of the attribute ID, for example:

```
FuelConsumption_0x108d
```

These custom fields are by default read-only. To allow field being written,
prefix it with `RW`, for example:

```
RWNormalRoomTemp_0x15e1
```

Note! Use `RW` naming only for fields that you have checked to be writable, see below.

You can use [`vitotrol`](https://github.com/maxatome/go-vitotrol) +
`rget all`, `bget` or `remote_attrs` actions to discover attributes and their read/write access
(german language skill needed :) ).

### Schedules (timesheets)

Switching-time programs are read and written by datapoint ID (decimal),
e.g. a WO1C circulation pump schedule `konf_schaltzeiten_zp_rw` 0x1d77 = 7543:

- publish anything to `<topic>/<device>/Schedule_7543/get` → the schedule is
  published **retained** on `<topic>/<device>/Schedule_7543` as
  `{"mon":[{"from":630,"to":900,"value":2}],...}` (times HHMM, `to` 0 = 24:00,
  `value` = the slot's Wert, e.g. pump cycle mode)
- publish JSON to `<topic>/<device>/Schedule_7543/set` (never retained), e.g.
  `{"mon-sun":[{"from":630,"to":900,"value":2}]}`, `{"mon-sun":[]}` = no slots.
  The current schedule is read first (its type is echoed on write), the write
  waits for the controller (~1 min) and the read-back is published.

## Run locally

Once your `vitotrol2mqtt.yml` is ready, you can launch:

```sh
vitotrol2mqtt -config vitotrol2mqtt.yml
```
## Docker
Alternatively, you can run the tool in a docker container.

```sh
docker-compose build --no-cache
docker-compose up 
```

That's all.

## Run on server, eg. Portainer

1. Portainer -> Images -> Build a new Image
2. Input name: vitotrol2mqtt:latest
3. Use URL option
4. URL: https://github.com/....git#master
5. Build the image
6. Go to Containers
7. Update the container image target to the above name, if needed
8. Deploy the container
9. Make sure the yaml file is in volume mount and pointed to containers /vitotrol2mqtt.yml path