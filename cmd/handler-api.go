// Copyright (c) 2015-2021 MinIO, Inc.
//
// This file is part of MinIO Object Storage stack
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package cmd

import (
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/mem"

	"github.com/minio/minio/internal/config/api"
	xioutil "github.com/minio/minio/internal/ioutil"
	"github.com/minio/minio/internal/logger"
	"github.com/minio/minio/internal/mcontext"
)

// 配置协议层的api，
type apiConfig struct {
	mu sync.RWMutex

	requestsDeadline time.Duration
	// 不配置，表示没有流控
	requestsPool     chan struct{}
	clusterDeadline  time.Duration
	listQuorum       string
	corsAllowOrigins []string
	// total drives per erasure set across pools.
	totalDriveCount     int
	replicationPriority string
	transitionWorkers   int

	staleUploadsExpiry          time.Duration
	staleUploadsCleanupInterval time.Duration
	deleteCleanupInterval       time.Duration
	enableODirect               bool
	gzipObjects                 bool
	rootAccess                  bool
	syncEvents                  bool
}

const cgroupLimitFile = "/sys/fs/cgroup/memory/memory.limit_in_bytes"

func cgroupLimit(limitFile string) (limit uint64) {
	buf, err := os.ReadFile(limitFile)
	if err != nil {
		return 9223372036854771712
	}
	limit, err = strconv.ParseUint(string(buf), 10, 64)
	if err != nil {
		return 9223372036854771712
	}
	return limit
}

func availableMemory() (available uint64) {
	available = 8 << 30 // Default to 8 GiB when we can't find the limits.

	if runtime.GOOS == "linux" {
		available = cgroupLimit(cgroupLimitFile)

		// No limit set, It's the highest positive signed 64-bit
		// integer (2^63-1), rounded down to multiples of 4096 (2^12),
		// the most common page size on x86 systems - for cgroup_limits.
		if available != 9223372036854771712 {
			// This means cgroup memory limit is configured.
			return
		} // no-limit set proceed to set the limits based on virtual memory.

	} // for all other platforms limits are based on virtual memory.

	memStats, err := mem.VirtualMemory()
	if err != nil {
		return
	}

	available = memStats.Available / 2
	return
}

func (t *apiConfig) init(cfg api.Config, setDriveCounts []int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	clusterDeadline := cfg.ClusterDeadline
	if clusterDeadline == 0 {
		clusterDeadline = 10 * time.Second
	}
	t.clusterDeadline = clusterDeadline
	corsAllowOrigin := cfg.CorsAllowOrigin
	if len(corsAllowOrigin) == 0 {
		corsAllowOrigin = []string{"*"}
	}
	t.corsAllowOrigins = corsAllowOrigin

	maxSetDrives := 0
	for _, setDriveCount := range setDriveCounts {
		t.totalDriveCount += setDriveCount
		if setDriveCount > maxSetDrives {
			maxSetDrives = setDriveCount
		}
	}

	var apiRequestsMaxPerNode int
	if cfg.RequestsMax <= 0 {
		maxMem := availableMemory()

		// max requests per node is calculated as
		// total_ram / ram_per_request
		// ram_per_request is (2MiB+128KiB) * driveCount \
		//    + 2 * 10MiB (default erasure block size v1) + 2 * 1MiB (default erasure block size v2)
		blockSize := xioutil.BlockSizeLarge + xioutil.BlockSizeSmall
		apiRequestsMaxPerNode = int(maxMem / uint64(maxSetDrives*blockSize+int(blockSizeV1*2+blockSizeV2*2)))
		if globalIsDistErasure {
			logger.Info("Automatically configured API requests per node based on available memory on the system: %d", apiRequestsMaxPerNode)
		}
	} else {
		apiRequestsMaxPerNode = cfg.RequestsMax
		if len(globalEndpoints.Hostnames()) > 0 {
			apiRequestsMaxPerNode /= len(globalEndpoints.Hostnames())
		}
	}

	if cap(t.requestsPool) != apiRequestsMaxPerNode {
		// Only replace if needed.
		// Existing requests will use the previous limit,
		// but new requests will use the new limit.
		// There will be a short overlap window,
		// but this shouldn't last long.
		t.requestsPool = make(chan struct{}, apiRequestsMaxPerNode)
	}
	t.requestsDeadline = cfg.RequestsDeadline
	listQuorum := cfg.ListQuorum
	if listQuorum == "" {
		listQuorum = "strict"
	}
	t.listQuorum = listQuorum
	if globalReplicationPool != nil &&
		cfg.ReplicationPriority != t.replicationPriority {
		globalReplicationPool.ResizeWorkerPriority(cfg.ReplicationPriority)
	}
	t.replicationPriority = cfg.ReplicationPriority
	if globalTransitionState != nil && cfg.TransitionWorkers != t.transitionWorkers {
		globalTransitionState.UpdateWorkers(cfg.TransitionWorkers)
	}
	t.transitionWorkers = cfg.TransitionWorkers

	t.staleUploadsExpiry = cfg.StaleUploadsExpiry
	t.staleUploadsCleanupInterval = cfg.StaleUploadsCleanupInterval
	t.deleteCleanupInterval = cfg.DeleteCleanupInterval
	t.enableODirect = cfg.EnableODirect
	t.gzipObjects = cfg.GzipObjects
	t.rootAccess = cfg.RootAccess
	t.syncEvents = cfg.SyncEvents
}

func (t *apiConfig) odirectEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.enableODirect
}

func (t *apiConfig) shouldGzipObjects() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.gzipObjects
}

func (t *apiConfig) permitRootAccess() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.rootAccess
}

func (t *apiConfig) getListQuorum() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.listQuorum == "" {
		return "strict"
	}

	return t.listQuorum
}

func (t *apiConfig) getCorsAllowOrigins() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.corsAllowOrigins) == 0 {
		return []string{"*"}
	}

	corsAllowOrigins := make([]string, len(t.corsAllowOrigins))
	copy(corsAllowOrigins, t.corsAllowOrigins)
	return corsAllowOrigins
}

func (t *apiConfig) getStaleUploadsCleanupInterval() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.staleUploadsCleanupInterval == 0 {
		return 6 * time.Hour // default 6 hours
	}

	return t.staleUploadsCleanupInterval
}

func (t *apiConfig) getStaleUploadsExpiry() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.staleUploadsExpiry == 0 {
		return 24 * time.Hour // default 24 hours
	}

	return t.staleUploadsExpiry
}

func (t *apiConfig) getDeleteCleanupInterval() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.deleteCleanupInterval == 0 {
		return 5 * time.Minute // every 5 minutes
	}

	return t.deleteCleanupInterval
}

func (t *apiConfig) getClusterDeadline() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.clusterDeadline == 0 {
		return 10 * time.Second
	}

	return t.clusterDeadline
}

func (t *apiConfig) getRequestsPoolCapacity() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return cap(t.requestsPool)
}

func (t *apiConfig) getRequestsPool() (chan struct{}, time.Duration) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.requestsPool == nil {
		return nil, time.Duration(0)
	}

	return t.requestsPool, t.requestsDeadline
}

// maxClients throttles the S3 API calls
// 限制请求的连接数目
func maxClients(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		globalHTTPStats.incS3RequestsIncoming()

		// 性能测试函数
		if r.Header.Get(globalObjectPerfUserMetadata) == "" {
			if val := globalServiceFreeze.Load(); val != nil {
				if unlock, ok := val.(chan struct{}); ok && unlock != nil {
					// Wait until unfrozen.
					select {
					case <-unlock:
					case <-r.Context().Done():
						// if client canceled we don't need to wait here forever.
						return
					}
				}
			}
		}

		// 初始化一个chan，定义了chan的cap，如果chan满了，就会阻塞
		// 初始化会给每个节点设计一个最大的连接数目，一般这个
		pool, deadline := globalAPIConfig.getRequestsPool()
		if pool == nil {
			f.ServeHTTP(w, r)
			return
		}

		globalHTTPStats.addRequestsInQueue(1)

		// trace content，做记录用的
		if tc, ok := r.Context().Value(mcontext.ContextTraceKey).(*mcontext.TraceCtxt); ok {
			tc.FuncName = "s3.MaxClients"
		}

		deadlineTimer := time.NewTimer(deadline)
		defer deadlineTimer.Stop()

		select {
		case pool <- struct{}{}:
			defer func() { <-pool }()
			globalHTTPStats.addRequestsInQueue(-1)
			f.ServeHTTP(w, r)
		case <-deadlineTimer.C:
			// Send a http timeout message
			writeErrorResponse(r.Context(), w,
				errorCodes.ToAPIErr(ErrTooManyRequests),
				r.URL)
			globalHTTPStats.addRequestsInQueue(-1)
			return
		case <-r.Context().Done():
			// When the client disconnects before getting the S3 handler
			// status code response, set the status code to 499 so this request
			// will be properly audited and traced.
			w.WriteHeader(499)
			globalHTTPStats.addRequestsInQueue(-1)
			return
		}
	}
}

func (t *apiConfig) getReplicationPriority() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.replicationPriority == "" {
		return "auto"
	}

	return t.replicationPriority
}

func (t *apiConfig) getTransitionWorkers() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.transitionWorkers <= 0 {
		return runtime.GOMAXPROCS(0) / 2
	}

	return t.transitionWorkers
}

func (t *apiConfig) isSyncEventsEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.syncEvents
}
