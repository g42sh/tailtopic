package tailtopic

import (
	"fmt"
	"os"
	"sync"

	"github.com/Shopify/sarama"
)

type consumer interface {
	consume(messages chan message, closing chan bool) error
}

type kafkaConsumer struct {
	topic   string
	offset  string
	broker  string
	decoder decoder
}

func (kc *kafkaConsumer) offsetVal() int64 {
	switch kc.offset {
	case "earliest":
		return sarama.OffsetOldest
	default:
		return sarama.OffsetNewest
	}
}

func (kc *kafkaConsumer) consume(messages chan message, closing chan bool) error {
	var wg sync.WaitGroup

	consumer, err := sarama.NewConsumer([]string{kc.broker}, nil)
	if err != nil {
		return err
	}

	partitionList, err := consumer.Partitions(kc.topic)
	if err != nil {
		return err
	}

	for _, partition := range partitionList {
		partitionConsumer, err := consumer.ConsumePartition(kc.topic, partition, kc.offsetVal())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start partition consumer: %s\n", err)
			continue
		}

		go func(pc sarama.PartitionConsumer) {
			<-closing
			pc.AsyncClose()
		}(partitionConsumer)

		wg.Add(1)
		go func(pc sarama.PartitionConsumer) {
			defer wg.Done()
			for msg := range pc.Messages() {
				val, err := kc.decoder.decode(msg.Value)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to decode message %s. Error: %v\n", string(msg.Value), err)
				}
				messages <- message{value: val}
			}
		}(partitionConsumer)
	}

	wg.Wait()

	close(messages)

	if err := consumer.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to close consumer: %s\n", err)
	}

	return nil
}
