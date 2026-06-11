package runtime

import (
	"errors"
	"os"
	"strings"

	"github.com/allcallall/backend/internal/mq"
	"github.com/allcallall/backend/internal/search"
	"github.com/allcallall/backend/internal/settlement"
)

func SearchServiceFromEnv() (*search.Service, string, error) {
	if raw := strings.TrimSpace(os.Getenv("ELASTICSEARCH_URL")); raw != "" {
		indexer, err := search.NewElasticsearchIndexer(search.ElasticsearchConfig{
			URL:      raw,
			Index:    os.Getenv("ELASTICSEARCH_INDEX"),
			Username: os.Getenv("ELASTICSEARCH_USERNAME"),
			Password: os.Getenv("ELASTICSEARCH_PASSWORD"),
		})
		if err != nil {
			return nil, "", err
		}
		return search.NewService(indexer), "elasticsearch", nil
	}
	return search.NewService(search.NewMemoryIndexer()), "memory", nil
}

func KafkaProducerFromEnv() (mq.Producer, bool, error) {
	brokers := mq.ParseBrokers(os.Getenv("KAFKA_BROKERS"))
	if len(brokers) == 0 {
		return nil, false, nil
	}
	return mq.NewKafkaProducer(brokers), true, nil
}

func KafkaConsumerFromEnv(topicFallback string, groupFallback string) (mq.Consumer, string, error) {
	brokers := mq.ParseBrokers(os.Getenv("KAFKA_BROKERS"))
	if len(brokers) == 0 {
		return nil, "", errors.New("KAFKA_BROKERS is required")
	}
	topic := strings.TrimSpace(os.Getenv("KAFKA_SETTLEMENT_TOPIC"))
	if topic == "" {
		topic = topicFallback
	}
	groupID := strings.TrimSpace(os.Getenv("KAFKA_CONSUMER_GROUP"))
	if groupID == "" {
		groupID = groupFallback
	}
	return mq.NewKafkaConsumer(mq.KafkaConfig{Brokers: brokers, Topic: topic, GroupID: groupID}), topic, nil
}

func SettlementTopicFromEnv() string {
	if topic := strings.TrimSpace(os.Getenv("KAFKA_SETTLEMENT_TOPIC")); topic != "" {
		return topic
	}
	return settlement.DefaultTopic
}
