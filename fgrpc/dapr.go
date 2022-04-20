package fgrpc

import (
	"context"
	"fmt"
	"strings"

	v1 "github.com/dapr/dapr/pkg/proto/common/v1"
	dapr "github.com/dapr/dapr/pkg/proto/runtime/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
)

const CAPABILITY_INVOKE = "invoke"
const CAPABILITY_STATE = "state"
const CAPABILITY_PUBSUB = "pubsub"
const TARGET_NOOP = "noop"
const TARGET_DAPR = "dapr"
const TARGET_APPCALLBACK = "appcallback"

type DaprGRPCRunnerResults struct {
	// common
	params            *DaprRequestParameters
	daprClient        dapr.DaprClient
	appCallbackClient dapr.AppCallbackClient

	// service invoke
	invokeRequest            *dapr.InvokeServiceRequest
	invokeAppCallbackRequest *v1.InvokeRequest

	// state
	getStateRequest *dapr.GetStateRequest

	// pub-sub
	publishEventRequest *dapr.PublishEventRequest
}

type DaprRequestParameters struct {
	capability string
	target     string
	method     string
	appId      string
	store      string

	extensions map[string]string
}

func (d *DaprGRPCRunnerResults) PrepareRequestAndConnection(o *GRPCRunnerOptions, conn *grpc.ClientConn) error {
	err := d.parseDaprParameters(o.UseDapr)
	if err != nil {
		return err
	}

	t := d.params.target
	c := d.params.capability
	err = fmt.Errorf("unsupported dapr load test: capability=%s, target=%s", c, t)

	if t == TARGET_NOOP {
		// do nothing for no-op
		return nil
	} else if t == TARGET_DAPR {
		d.daprClient = dapr.NewDaprClient(conn)
		if c == CAPABILITY_INVOKE {
			err = d.prepareRequest4Invoke(o)
		} else if c == CAPABILITY_STATE {
			err = d.prepareRequest4State(o)
		} else if c == CAPABILITY_PUBSUB {
			err = d.prepareRequest4PubSub(o)
		}
	} else if t == TARGET_APPCALLBACK {
		d.appCallbackClient = dapr.NewAppCallbackClient(conn)
		if c == CAPABILITY_INVOKE {
			err = d.prepareRequest4InvokeAppCallback(o)
		}
	}

	return err
}

func (d *DaprGRPCRunnerResults) prepareRequest4Invoke(o *GRPCRunnerOptions) error {
	method := d.params.method
	if method == "" {
		return fmt.Errorf("method is required for load test")
	}

	d.invokeRequest = &dapr.InvokeServiceRequest{
		Id: d.params.appId,
		Message: &v1.InvokeRequest{
			Method:      method,
			ContentType: "text/plain",
		},
	}

	if len(o.Payload) > 0 {
		d.invokeRequest.Message.Data = &anypb.Any{Value: []byte(o.Payload)}
	} else {
		d.invokeRequest.Message.Data = &anypb.Any{Value: []byte{}}
	}
	return nil
}

func (d *DaprGRPCRunnerResults) prepareRequest4State(o *GRPCRunnerOptions) error {
	method := d.params.method
	store := d.params.store
	key := d.params.extensions["key"]
	if method == "" {
		return fmt.Errorf("method is required for state load test")
	}
	if store == "" {
		return fmt.Errorf("store is required for state load test")
	}
	if key == "" {
		return fmt.Errorf("key is required for state load test")
	}

	switch method {
	case "get":
		d.getStateRequest = &dapr.GetStateRequest{
			StoreName: store,
			Key:       key,
		}
	default:
		return fmt.Errorf("unsupported method of state load test: method=%s", method)
	}

	return nil
}

func (d *DaprGRPCRunnerResults) prepareRequest4PubSub(o *GRPCRunnerOptions) error {
	method := d.params.method
	store := d.params.store
	topic := d.params.extensions["topic"]
	contentType := d.params.extensions["contenttype"]
	if method == "" {
		return fmt.Errorf("method is required for pubsub load test")
	}
	if store == "" {
		return fmt.Errorf("store(pubsub name) is required for pubsub load test")
	}
	if topic == "" {
		return fmt.Errorf("topic is required for pubsub load test")
	}

	switch method {
	case "publish":
		d.publishEventRequest = &dapr.PublishEventRequest{
			PubsubName:      store,
			Topic:           topic,
			DataContentType: contentType,
		}
		if len(o.Payload) > 0 {
			d.publishEventRequest.Data = []byte(o.Payload)
		} else {
			d.publishEventRequest.Data = []byte{}
		}
	default:
		return fmt.Errorf("unsupported method of pubsub load test: method=%s", method)
	}

	return nil
}

func (d *DaprGRPCRunnerResults) prepareRequest4InvokeAppCallback(o *GRPCRunnerOptions) error {
	method := d.params.method
	if method == "" {
		return fmt.Errorf("method is required for load test")
	}

	d.invokeAppCallbackRequest = &v1.InvokeRequest{
		Method:      method,
		ContentType: "text/plain",
	}
	if len(o.Payload) > 0 {
		d.invokeAppCallbackRequest.Data = &anypb.Any{Value: []byte(o.Payload)}
	} else {
		d.invokeAppCallbackRequest.Data = &anypb.Any{Value: []byte{}}
	}
	return nil
}

func (d *DaprGRPCRunnerResults) RunTest() error {
	t := d.params.target
	c := d.params.capability
	if t == TARGET_NOOP {
		// do nothing for no-op
		return nil
	}

	err := fmt.Errorf("unsupported dapr load test: capability=%s, target=%s", c, t)

	if c == CAPABILITY_INVOKE {
		if t == TARGET_DAPR {
			_, err = d.daprClient.InvokeService(context.Background(), d.invokeRequest)
		} else if t == TARGET_APPCALLBACK {
			_, err = d.appCallbackClient.OnInvoke(context.Background(), d.invokeAppCallbackRequest)
		}
	} else if c == CAPABILITY_STATE {
		if t == TARGET_DAPR {
			_, err = d.daprClient.GetState(context.Background(), d.getStateRequest)
		}
	} else if c == CAPABILITY_STATE {
		if t == TARGET_DAPR {
			_, err = d.daprClient.GetState(context.Background(), d.getStateRequest)
		}
	} else if c == CAPABILITY_PUBSUB {
		if t == TARGET_DAPR {
			_, err = d.daprClient.PublishEvent(context.Background(), d.publishEventRequest)
		}
	}

	return err
}

func (d *DaprGRPCRunnerResults) parseDaprParameters(params string) error {
	d.params = &DaprRequestParameters{extensions: make(map[string]string)}

	kvs := strings.Split(params, ",")
	for _, kv := range kvs {
		kv := strings.Split(kv, "=")
		k := strings.TrimSpace(kv[0])
		v := strings.TrimSpace(kv[1])
		switch k {
		case "capability":
			d.params.capability = v
		case "target":
			d.params.target = v
		case "method":
			d.params.method = v
		case "appid":
			d.params.appId = v
		case "store":
			d.params.store = v
		default:
			d.params.extensions[k] = v
		}
	}

	return nil
}