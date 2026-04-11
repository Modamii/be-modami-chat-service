package kafka

import (
	"crypto/tls"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

type KafkaConfig struct {
	Brokers             []string      `json:"brokers" yaml:"brokers"`
	ClientID            string        `json:"clientId" yaml:"clientId"`
	ConsumerGroupID     string        `json:"consumerGroupId" yaml:"consumerGroupId"`
	ProducerOnlyMode    bool          `json:"producerOnlyMode" yaml:"producerOnlyMode"`
	ConnectionTimeout   time.Duration `json:"connectionTimeout" yaml:"connectionTimeout"`
	ConsumerMaxBytes    int32         `json:"consumerMaxBytes" yaml:"consumerMaxBytes"`
	ConsumerConcurrency int           `json:"consumerConcurrency" yaml:"consumerConcurrency"`
	SSL                 *SSLConfig    `json:"ssl,omitempty" yaml:"ssl,omitempty"`
	SASL                *SASLConfig   `json:"sasl,omitempty" yaml:"sasl,omitempty"`
}

type SSLConfig struct {
	CA   string `json:"ca" yaml:"ca"`
	Key  string `json:"key" yaml:"key"`
	Cert string `json:"cert" yaml:"cert"`
}

type SASLConfig struct {
	Mechanism string `json:"mechanism" yaml:"mechanism"`
	Username  string `json:"username" yaml:"username"`
	Password  string `json:"password" yaml:"password"`
}

func (kc *KafkaConfig) ToFranzGoOpts() ([]kgo.Opt, error) {
	if kc.ConnectionTimeout == 0 {
		kc.ConnectionTimeout = 30 * time.Second
	}
	if kc.ConsumerMaxBytes == 0 {
		kc.ConsumerMaxBytes = 10000
	}

	opts := []kgo.Opt{
		kgo.SeedBrokers(kc.Brokers...),
		kgo.ClientID(kc.ClientID),
		kgo.DialTimeout(kc.ConnectionTimeout),
		kgo.FetchMaxBytes(kc.ConsumerMaxBytes),
	}

	if !kc.ProducerOnlyMode && kc.ConsumerGroupID != "" {
		opts = append(opts, kgo.ConsumerGroup(kc.ConsumerGroupID))
	}

	if kc.SSL != nil {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: false,
		}
		opts = append(opts, kgo.DialTLSConfig(tlsConfig))
	}

	if kc.SASL != nil {
		var mechanism sasl.Mechanism
		switch kc.SASL.Mechanism {
		case "PLAIN":
			mechanism = plain.Auth{
				User: kc.SASL.Username,
				Pass: kc.SASL.Password,
			}.AsMechanism()
		case "SCRAM-SHA-256":
			mechanism = scram.Auth{
				User: kc.SASL.Username,
				Pass: kc.SASL.Password,
			}.AsSha256Mechanism()
		case "SCRAM-SHA-512":
			mechanism = scram.Auth{
				User: kc.SASL.Username,
				Pass: kc.SASL.Password,
			}.AsSha512Mechanism()
		default:
			return nil, fmt.Errorf("unsupported SASL mechanism: %s", kc.SASL.Mechanism)
		}
		opts = append(opts, kgo.SASL(mechanism))
	}

	return opts, nil
}
