package routing

import (
	"container/heap"

	"walking-aware-nav/internal/model"
)

// QueueItem is an A* frontier entry.
type QueueItem struct {
	State     StateKey
	Cost      SearchCost
	Parent    *QueueItem
	Edge      model.Edge
	Primary   float64
	Secondary float64
	Index     int
}

// PriorityQueue implements heap.Interface as a min-priority queue.
type PriorityQueue []*QueueItem

var _ heap.Interface = (*PriorityQueue)(nil)

func (queue PriorityQueue) Len() int {
	return len(queue)
}

func (queue PriorityQueue) Less(i, j int) bool {
	if queue[i].Primary != queue[j].Primary {
		return queue[i].Primary < queue[j].Primary
	}
	if queue[i].Secondary != queue[j].Secondary {
		return queue[i].Secondary < queue[j].Secondary
	}
	if queue[i].Cost.TravelTime != queue[j].Cost.TravelTime {
		return queue[i].Cost.TravelTime < queue[j].Cost.TravelTime
	}
	if queue[i].Cost.WalkingUnits != queue[j].Cost.WalkingUnits {
		return queue[i].Cost.WalkingUnits < queue[j].Cost.WalkingUnits
	}
	if queue[i].State.Node != queue[j].State.Node {
		return queue[i].State.Node < queue[j].State.Node
	}
	if queue[i].State.OnTransit != queue[j].State.OnTransit {
		return !queue[i].State.OnTransit
	}
	return queue[i].State.LastService < queue[j].State.LastService
}

func (queue PriorityQueue) Swap(i, j int) {
	queue[i], queue[j] = queue[j], queue[i]
	queue[i].Index = i
	queue[j].Index = j
}

func (queue *PriorityQueue) Push(value any) {
	item := value.(*QueueItem)
	item.Index = len(*queue)
	*queue = append(*queue, item)
}

func (queue *PriorityQueue) Pop() any {
	old := *queue
	last := len(old) - 1
	item := old[last]
	old[last] = nil
	item.Index = -1
	*queue = old[:last]
	return item
}
