package distributor

import (
	"context"
	"fmt"
	"log"

	hubGrpc "github.com/facebookincubator/prometheus-edge-hub/grpc"
	dto "github.com/prometheus/client_model/go"
	"github.com/serialx/hashring"
	"github.com/thoas/go-funk"
	"google.golang.org/grpc"
)

type Distributor struct {
	keyLabel     string
	edgeHubArray []string
	connections  map[string]*grpc.ClientConn
	ring 		 *hashring.HashRing
}

func NewDistributor(keyLabel string, edgeHubArray []string) *Distributor {
	conns := map[string]*grpc.ClientConn{}
	for _, hub := range edgeHubArray {
		conn, err := grpc.Dial(hub, grpc.WithInsecure())
		if err != nil {
			log.Fatalf("Error connecting to %s: %v", hub, err)
		}
		conns[hub] = conn
	}
	ring := hashring.New(edgeHubArray)
	fmt.Printf("Ring nodes: %v\n", edgeHubArray)
	return &Distributor{
		keyLabel:     keyLabel,
		edgeHubArray: edgeHubArray,
		connections:  conns,
		ring:         ring,
	}
}

func (d *Distributor) ReceiveGRPC(families []*dto.MetricFamily) {
	distSet := map[string]map[string]*dto.MetricFamily{}

	for _, hub := range d.edgeHubArray {
		distSet[hub] = make(map[string]*dto.MetricFamily, 0)
	}

	fmt.Printf("Received %d families\n", len(families))
	for _, fam := range families {
		name := fam.GetName()
		for _, met := range fam.Metric {
			labelVal := getKeyLabelValue(met, d.keyLabel)
			hub, ok := d.ring.GetNode(labelVal)
			if !ok {
				log.Printf("Hash position not found for %s", met.String())
			}
			if _, ok := distSet[hub][name]; ok {
				distSet[hub][name].Metric = append(distSet[hub][name].Metric, met)
			} else {
				//fmt.Printf("Metric %s placed in %s\n", met.String(), hub)
				distSet[hub][name] = &dto.MetricFamily{
					Name:                 fam.Name,
					Help:                 fam.Help,
					Type:                 fam.Type,
					Metric:               []*dto.Metric{met},
				}
			}
		}
	}
	for hub := range distSet {
		famsInHub := len(distSet[hub])
		fmt.Printf("Sending %d families to %s\n", famsInHub, hub)
	}
	err := sendToHubs(distSet, d.connections)
	if err != nil {
		log.Fatalf("Error sending: %v", err)
	}
}

func getKeyLabelValue(metric *dto.Metric, keyLabel string) string {
	for _, label := range metric.GetLabel() {
		if *label.Name == keyLabel {
			return label.GetValue()
		}
	}
	return ""
}

func sendToHubs(distSet map[string]map[string]*dto.MetricFamily, connections map[string]*grpc.ClientConn) error {
	for hub, families := range distSet {
		client := hubGrpc.NewMetricsControllerClient(connections[hub])
		fams := funk.Values(families).([]*dto.MetricFamily)
		_, err := client.Collect(context.Background(), &hubGrpc.MetricFamilies{Families: fams})
		if err != nil {
			return err
		}
	}
	return nil
}
