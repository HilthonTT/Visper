package broker

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// ConsumerGroup coordinates multiple consumers in a group
type ConsumerGroup struct {
	broker   *Broker
	groupID  string
	members  map[string]*GroupMember
	topics   map[string][]int // topic -> partitions
	mu       sync.Mutex
	strategy PartitionAssignmentStrategy
}

// GroupMember represents a member of a consumer group
type GroupMember struct {
	id            string
	topics        []string
	partitions    map[string][]int // topic -> partitions
	lastHeartbeat time.Time
}

// PartitionAssignmentStrategy determines how partitions are assigned to consumers
type PartitionAssignmentStrategy func(members map[string]*GroupMember, topics map[string][]int) map[string]map[string][]int

// RangeAssignmentStrategy assigns partitions to consumers using the range strategy
func RangeAssignmentStrategy(members map[string]*GroupMember, topics map[string][]int) map[string]map[string][]int {
	// Map of member ID -> topic -> partitions
	assignments := make(map[string]map[string][]int)

	// Initialize assignments map
	for memberID, member := range members {
		assignments[memberID] = make(map[string][]int)

		// Initialize empty arrays for each topic the member is subscribed to
		for _, topic := range member.topics {
			assignments[memberID][topic] = []int{}
		}
	}

	// Create a mapping of topics to interested members
	topicMembers := make(map[string][]string)
	for topic, _ := range topics {
		topicMembers[topic] = []string{}
	}

	for memberID, member := range members {
		for _, topic := range member.topics {
			if _, exists := topics[topic]; exists {
				topicMembers[topic] = append(topicMembers[topic], memberID)
			}
		}
	}

	// Assign partitions for each topic
	for topic, partitions := range topics {
		interestedMembers := topicMembers[topic]
		if len(interestedMembers) == 0 {
			continue
		}

		// Sort member IDs for deterministic assignment
		sort.Strings(interestedMembers)

		// Calculate partitions per member
		numPartitions := len(partitions)
		numMembers := len(interestedMembers)

		partitionsPerMember := numPartitions / numMembers
		remainder := numPartitions % numMembers

		start := 0
		for i, memberID := range interestedMembers {
			// Calculate member's partition count (distribute remainder)
			count := partitionsPerMember
			if i < remainder {
				count++
			}

			end := start + count
			if end > numPartitions {
				end = numPartitions
			}

			// Assign partitions to member
			for j := start; j < end; j++ {
				assignments[memberID][topic] = append(assignments[memberID][topic], partitions[j])
			}

			start = end
		}
	}

	return assignments
}

// NewConsumerGroup creates a new consumer group
func NewConsumerGroup(broker *Broker, groupID string) *ConsumerGroup {
	return &ConsumerGroup{
		broker:   broker,
		groupID:  groupID,
		members:  make(map[string]*GroupMember),
		topics:   make(map[string][]int),
		strategy: RangeAssignmentStrategy,
	}
}

// Join adds a consumer to the group
func (cg *ConsumerGroup) Join(memberID string, topics []string) (map[string][]int, error) {
	cg.mu.Lock()
	defer cg.mu.Unlock()

	// Check if member already exists
	if member, exists := cg.members[memberID]; exists {
		// Update topics
		member.topics = topics
		member.lastHeartbeat = time.Now()
	} else {
		// Create new member
		cg.members[memberID] = &GroupMember{
			id:            memberID,
			topics:        topics,
			partitions:    make(map[string][]int),
			lastHeartbeat: time.Now(),
		}
	}

	// Load topics
	for _, topicName := range topics {
		if _, exists := cg.topics[topicName]; !exists {
			// Get topic
			topic, err := cg.broker.GetTopic(topicName)
			if err != nil {
				return nil, fmt.Errorf("failed to get topic %s: %w", topicName, err)
			}

			// Get partitions
			partitions := make([]int, len(topic.partitions))
			for i := range topic.partitions {
				partitions[i] = i
			}

			cg.topics[topicName] = partitions
		}
	}

	// Rebalance group
	assignments := cg.rebalance()

	return assignments[memberID], nil
}

// Leave removes a consumer from the group
func (cg *ConsumerGroup) Leave(memberID string) error {
	cg.mu.Lock()
	defer cg.mu.Unlock()

	// Remove member
	delete(cg.members, memberID)

	// Rebalance group
	cg.rebalance()

	return nil
}

// Heartbeat updates a member's last heartbeat time
func (cg *ConsumerGroup) Heartbeat(memberID string) error {
	cg.mu.Lock()
	defer cg.mu.Unlock()

	// Check if member exists
	member, exists := cg.members[memberID]
	if !exists {
		return fmt.Errorf("member %s does not exist", memberID)
	}

	// Update heartbeat
	member.lastHeartbeat = time.Now()

	return nil
}

// CheckHeartbeats removes members that haven't sent a heartbeat recently
func (cg *ConsumerGroup) CheckHeartbeats() {
	cg.mu.Lock()
	defer cg.mu.Unlock()

	// Find expired members
	expiredMembers := []string{}
	deadline := time.Now().Add(-30 * time.Second)

	for memberID, member := range cg.members {
		if member.lastHeartbeat.Before(deadline) {
			expiredMembers = append(expiredMembers, memberID)
		}
	}

	// Remove expired members
	for _, memberID := range expiredMembers {
		delete(cg.members, memberID)
	}

	// Rebalance if any members were removed
	if len(expiredMembers) > 0 {
		cg.rebalance()
	}
}

// rebalance reassigns partitions to consumers
func (cg *ConsumerGroup) rebalance() map[string]map[string][]int {
	// Assign partitions using strategy
	assignments := cg.strategy(cg.members, cg.topics)

	// Update member assignments
	for memberID, topicPartitions := range assignments {
		member, exists := cg.members[memberID]
		if !exists {
			continue
		}

		member.partitions = topicPartitions
	}

	return assignments
}
