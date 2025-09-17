package processor

import (
	"fmt"
	"sync"
	"time"

	"github.com/san-kum/motorcycle-cv/server/models"
)

type ProcessingQueue struct {
	items      chan *QueueItem
	workers    int
	workerFunc func(*QueueItem)
	wg         sync.WaitGroup
	shutdown   chan struct{}
	isRunning  bool
	mutex      sync.RWMutex
}

type QueueItem struct {
	Request    *models.FrameRequest
	ResultChan chan *ProcessingResult
	StartTime  time.Time
	Priority   int // Higher values = higher priority
}

type ProcessingResult struct {
	Analysis *models.AnalysisResult
	Error    error
}

func NewProcessingQueue(queueSize, workers int, workerFunc func(*QueueItem)) *ProcessingQueue {
	queue := &ProcessingQueue{
		items:      make(chan *QueueItem, queueSize),
		workers:    workers,
		workerFunc: workerFunc,
		shutdown:   make(chan struct{}),
		isRunning:  true,
	}

	for i := 0; i < workers; i++ {
		queue.wg.Add(1)
		go queue.worker(i)
	}

	return queue
}

func (pq *ProcessingQueue) worker(id int) {
	defer pq.wg.Done()

	for {
		select {
		case item := <-pq.items:
			if item != nil {
				func() {
					defer func() {
						if r := recover(); r != nil {
							select {
							case item.ResultChan <- &ProcessingResult{
								Error: fmt.Errorf("worker panic: %v", r),
							}:
							default:
							}
						}
					}()

					pq.workerFunc(item)
				}()
			}
		case <-pq.shutdown:
			return
		}
	}
}

func (pq *ProcessingQueue) Enqueue(item *QueueItem) bool {
	pq.mutex.RLock()
	if !pq.isRunning {
		pq.mutex.RUnlock()
		return false
	}
	pq.mutex.RUnlock()

	select {
	case pq.items <- item:
		return true
	default:
		return false
	}
}

func (pq *ProcessingQueue) Size() int {
	return len(pq.items)
}

func (pq *ProcessingQueue) Capacity() int {
	return cap(pq.items)
}

func (pq *ProcessingQueue) IsRunning() bool {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	return pq.isRunning
}

func (pq *ProcessingQueue) Workers() int {
	return pq.workers
}

func (pq *ProcessingQueue) Shutdown(timeout time.Duration) error {
	pq.mutex.Lock()
	if !pq.isRunning {
		pq.mutex.Unlock()
		return nil
	}
	pq.isRunning = false
	pq.mutex.Unlock()

	close(pq.shutdown)

	done := make(chan struct{})
	go func() {
		pq.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		close(pq.items)
		return nil
	case <-time.After(timeout):
		close(pq.items)
		return fmt.Errorf("shutdown timeout exceeded")
	}
}

func (pq *ProcessingQueue) DrainQueue() int {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()

	drained := 0

	for {
		select {
		case item := <-pq.items:
			if item != nil {
				select {
				case item.ResultChan <- &ProcessingResult{
					Error: fmt.Errorf("processing cancelled - queue shutting down"),
				}:
				default:
				}
				drained++
			}
		default:
			return drained
		}
	}
}

func (pq *ProcessingQueue) GetQueueStats() QueueStats {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()

	return QueueStats{
		CurrentSize:        pq.Size(),
		MaxCapacity:        pq.Capacity(),
		ActiveWorkers:      pq.workers,
		IsRunning:          pq.isRunning,
		UtilizationPercent: float64(pq.Size()) / float64(pq.Capacity()) * 100,
	}
}

type QueueStats struct {
	CurrentSize        int     `json:"current_size"`
	MaxCapacity        int     `json:"max_capacity"`
	ActiveWorkers      int     `json:"active_workers"`
	IsRunning          bool    `json:"is_running"`
	UtilizationPercent float64 `json:"utilization_percent"`
}

type PriorityQueue struct {
	items []*QueueItem
	mutex sync.RWMutex
}

func NewPriorityQueue() *PriorityQueue {
	return &PriorityQueue{
		items: make([]*QueueItem, 0),
	}
}

func (pq *PriorityQueue) Push(item *QueueItem) {
	pq.mutex.Lock()
	defer pq.mutex.Unlock()

	inserted := false
	for i, existing := range pq.items {
		if item.Priority > existing.Priority {
			pq.items = append(pq.items[:i], append([]*QueueItem{item}, pq.items[i:]...)...)
			inserted = true
			break
		}
	}

	if !inserted {
		pq.items = append(pq.items, item)
	}
}

func (pq *PriorityQueue) Pop() *QueueItem {
	pq.mutex.Lock()
	defer pq.mutex.Unlock()

	if len(pq.items) == 0 {
		return nil
	}

	item := pq.items[0]
	pq.items = pq.items[1:]
	return item
}

func (pq *PriorityQueue) Len() int {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	return len(pq.items)
}

func (pq *PriorityQueue) Clear() {
	pq.mutex.Lock()
	defer pq.mutex.Unlock()
	pq.items = pq.items[:0]
}
