package main

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/cockroachdb/cockroach/pkg/cmd/roachtest/operations"
	"github.com/cockroachdb/cockroach/pkg/cmd/roachtest/registry"
	"github.com/cockroachdb/cockroach/pkg/cmd/roachtest/roachtestflags"
	"github.com/cockroachdb/cockroach/pkg/roachprod"
	"github.com/cockroachdb/cockroach/pkg/roachprod/logger"
	"github.com/cockroachdb/cockroach/pkg/util/randutil"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

var originalOpSpecs map[string]*registry.OperationSpec

// OperationsWorkflow is the Temporal workflow that manages the execution of operations
func OperationsWorkflow(ctx workflow.Context, clusterName string) (string, error) {
	l := workflow.GetLogger(ctx)
	if err := roachprod.LoadClusters(); err != nil {
		return "", err
	}
	c, err := getCachedCluster(clusterName)
	if err != nil {
		return "", err
	}

	_, seed := randutil.NewTestRand()
	// Prepare workflow input
	input := &OperationsWorkflowInput{
		ClusterName:        clusterName,
		NodeCount:          c.VMs.Len(),
		Seed:               seed,
		Parallelism:        min(roachtestflags.OperationParallelism, roachtestflags.MaxOperationParallelism),
		DatadogTags:        getDatadogTags(),
		WaitBeforeNextExec: roachtestflags.WaitBeforeNextExecution,
	}

	if roachtestflags.WorkloadCluster != "" {
		workloadCluster, err := getCachedCluster(roachtestflags.WorkloadCluster)
		if err != nil {
			return "", err
		}
		input.WorkloadClusterName = workloadCluster.Name
		input.WorkloadNodes = workloadCluster.VMs.Len()
	}
	l.Info("Starting operations workflow", "parallelism", input.Parallelism)

	// Yield control at the beginning of the workflow to prevent deadlock
	_ = workflow.Sleep(ctx, time.Millisecond)

	// Create a selector to handle signals and child workflows
	selector := workflow.NewSelector(ctx)

	// Flag to indicate if the workflow should terminate
	shouldTerminate := false

	// Add signal handler for cancellation
	signalChan := workflow.GetSignalChannel(ctx, "cancel")
	selector.AddReceive(signalChan, func(c workflow.ReceiveChannel, more bool) {
		var signal string
		c.Receive(ctx, &signal)
		l.Info("Received cancel signal, stopping workflow")
		shouldTerminate = true
	})

	// Track running operations
	runningOps := make(map[string]struct{})
	lastRun := make(map[string]time.Time)

	// Create a channel to send operation requests to workers
	requestChan := workflow.NewChannel(ctx)

	// Start a fixed pool of workers
	workerStatuses := make([]bool, input.Parallelism) // true means worker is busy
	for i := 0; i < input.Parallelism; i++ {
		workerID := i + 1
		workerStatuses[i] = false // initially all workers are free

		// Start each worker in its own goroutine
		workflow.Go(ctx, func(ctx workflow.Context) {
			for {
				// Wait for a request
				var request ExecuteOperationRequest
				requestChan.Receive(ctx, &request)

				// Mark worker as busy
				workerStatuses[workerID-1] = true

				l.Info("Worker processing request", "workerID", workerID, "regex", request.OperationRegex)

				// Process the request
				runWorker(ctx, input, &request, workerID, runningOps, lastRun)

				// If RunInInterval is set, schedule the same request to run again after the interval
				if request.RunInInterval > 0 {
					reqCopy := request // Make a copy to avoid issues with variable capture

					// If MaxRepetitions is set and greater than 0, decrement it for the next run
					if reqCopy.MaxRepetitions > 0 {
						reqCopy.MaxRepetitions--
						workflow.Go(ctx, func(ctx workflow.Context) {
							_ = workflow.Sleep(ctx, reqCopy.RunInInterval)
							l.Info("Scheduling operation to run again", "workerID", workerID, "regex", reqCopy.OperationRegex,
								"interval", reqCopy.RunInInterval, "remainingRepetitions", reqCopy.MaxRepetitions)
							// Send the modified request back to the channel
							requestChan.Send(ctx, reqCopy)
						})
					} else if reqCopy.MaxRepetitions == 0 {
						// MaxRepetitions is 0, which means unlimited repetitions
						workflow.Go(ctx, func(ctx workflow.Context) {
							_ = workflow.Sleep(ctx, reqCopy.RunInInterval)
							l.Info("Scheduling operation to run again (unlimited)", "workerID", workerID, "regex", reqCopy.OperationRegex,
								"interval", reqCopy.RunInInterval)
							// Send the same request back to the channel
							requestChan.Send(ctx, reqCopy)
						})
					}
					// If MaxRepetitions is negative, don't schedule again
				}

				// Mark worker as free
				workerStatuses[workerID-1] = false

				l.Info("Worker completed request", "workerID", workerID)
			}
		})
	}

	// Add signal handler for operation execution requests
	executeOpSignalChan := workflow.GetSignalChannel(ctx, ExecuteOperationSignal)
	selector.AddReceive(executeOpSignalChan, func(c workflow.ReceiveChannel, more bool) {
		var request ExecuteOperationRequest
		c.Receive(ctx, &request)

		l.Info("Received execute operation signal", "regex", request.OperationRegex, "cluster", clusterName)

		// Find a free worker
		freeWorkerFound := false
		for i := 0; i < input.Parallelism; i++ {
			if !workerStatuses[i] {
				// Send the request to the worker
				requestChan.Send(ctx, request)
				freeWorkerFound = true
				break
			}
		}

		if !freeWorkerFound {
			l.Info("No free workers available, request will be queued")
			// Queue the request to be processed when a worker becomes available
			workflow.Go(ctx, func(ctx workflow.Context) {
				// Keep trying to find a free worker
				for {
					// Sleep for a short time before checking again
					_ = workflow.Sleep(ctx, time.Second)

					// Check if any worker is free
					for i := 0; i < input.Parallelism; i++ {
						if !workerStatuses[i] {
							// Send the request to the worker
							requestChan.Send(ctx, request)
							return
						}
					}
				}
			})
		}
	})

	// Wait for signals in a loop to allow receiving the same signal multiple times
	for !shouldTerminate {
		selector.Select(ctx)
		// After processing a signal, continue listening for more signals if we shouldn't terminate
		if !shouldTerminate {
			l.Info("Processed a signal, continuing to listen for more signals")
		}
	}

	l.Info("Workflow terminating")
	return "Operations workflow completed", nil
}

func populateOpsSpec() {
	r := makeTestRegistry()
	operations.RegisterOperations(r)
	// Store all operation specs for later use
	originalOpSpecs = make(map[string]*registry.OperationSpec)
	allOpSpecs := r.AllOperations()
	for _, spec := range allOpSpecs {
		// Store the original spec by name
		specCopy := spec
		originalOpSpecs[spec.Name] = &specCopy
	}
}

// runWorker is the worker function that selects and runs operations
func runWorker(
	ctx workflow.Context,
	input *OperationsWorkflowInput,
	request *ExecuteOperationRequest,
	workerIdx int,
	runningOps map[string]struct{},
	lastRun map[string]time.Time,
) {
	l := workflow.GetLogger(ctx)
	l.Info("Worker processing operation", "workerIdx", workerIdx, "regex", request.OperationRegex)

	// Create a workflow-deterministic random source
	seed := input.Seed + int64(workerIdx)

	// Check if workflow is replaying and yield control to prevent deadlock if needed
	if workflow.IsReplaying(ctx) {
		// Yield control during replay to prevent deadlock
		_ = workflow.Sleep(ctx, time.Millisecond)
		// Don't continue here, as it creates non-deterministic behavior
	}

	opsSpecs, err := filterOpsToRun(request.OperationRegex)
	if err != nil {
		l.Error("Operation failed", "operation regex", request.OperationRegex, "error", err)
		return
	}

	// Select an operation to run
	opSpec := selectOperationToRun(ctx, opsSpecs, runningOps, lastRun, seed, workerIdx, input.WaitBeforeNextExec)
	if opSpec == nil {
		l.Info("Couldn't find candidate operation to run", "workerIdx", workerIdx)
		return
	}

	// Mark operation as running
	runningOps[opSpec.NamePrefix()] = struct{}{}

	// Run the operation as an activity
	activityInput := OperationActivityInput{
		ClusterName:         input.ClusterName,
		NodeCount:           input.NodeCount,
		OperationSpec:       opSpec,
		Seed:                seed,
		WorkerIdx:           workerIdx,
		WorkloadClusterName: input.WorkloadClusterName,
		WorkloadNodes:       input.WorkloadNodes,
		DatadogTags:         input.DatadogTags,
	}

	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: opSpec.Timeout,
		// Don't retry operations
	}

	activityCtx := workflow.WithActivityOptions(ctx, activityOptions)
	err = workflow.ExecuteActivity(activityCtx, RunOperationActivity, activityInput).Get(activityCtx, nil)

	// Update operation status
	delete(runningOps, opSpec.NamePrefix())
	lastRun[opSpec.NamePrefix()] = workflow.Now(ctx)

	if err != nil {
		l.Error("Operation failed", "operation", opSpec.Name, "error", err)
	} else {
		l.Info("Operation completed successfully", "operation", opSpec.Name)
	}

	// Note: RunInInterval handling is now done in the main workflow
}

// selectOperationToRun picks one operation to run that hasn't been run for at least waitBeforeNextExecution time.
// This is a workflow-deterministic version of the original selectOperationToRun function.
func selectOperationToRun(
	ctx workflow.Context,
	opSpecs []SerializableOperationSpec,
	runningOps map[string]struct{},
	lastRun map[string]time.Time,
	seed int64,
	workerID int,
	waitBeforeNextExec time.Duration,
) *SerializableOperationSpec {
	now := workflow.Now(ctx)

	// Yield control to prevent potential deadlock
	_ = workflow.Sleep(ctx, time.Millisecond)

	// Filter operations that can be run
	var candidates []SerializableOperationSpec
	for _, opSpec := range opSpecs {
		// Skip operations that are already running
		if _, ok := runningOps[opSpec.NamePrefix()]; ok {
			continue
		}

		// Check if enough time has passed since last run
		lastRunTime, ok := lastRun[opSpec.NamePrefix()]
		if ok && now.Sub(lastRunTime) < waitBeforeNextExec {
			continue
		}

		// Check concurrency constraints
		if opSpec.CanRunConcurrently == registry.OperationCannotRunConcurrently {
			if len(runningOps) > 0 {
				continue
			}
		} else if opSpec.CanRunConcurrently == registry.OperationCannotRunConcurrentlyWithItself {
			if _, ok := runningOps[opSpec.NamePrefix()]; ok {
				continue
			}
		}

		candidates = append(candidates, opSpec)
	}

	// Yield control again if we processed a lot of operations
	if len(opSpecs) > 50 {
		_ = workflow.Sleep(ctx, time.Millisecond)
	}

	if len(candidates) == 0 {
		return nil
	}

	// Select a random candidate
	idx := int(seed+int64(workerID)) % len(candidates)
	return &candidates[idx]
}

// RunOperationActivity is the activity that executes a single operation
func RunOperationActivity(ctx context.Context, input OperationActivityInput) error {
	// Create a logger
	l, err := logger.RootLogger("", logger.NoTee)
	if err != nil {
		return err
	}

	// Look up the original OperationSpec with the Run function
	origOpSpec, ok := originalOpSpecs[input.OperationSpec.Name]
	origOpSpec.WaitBeforeCleanup = time.Minute
	if !ok {
		return fmt.Errorf("could not find original OperationSpec for %s", input.OperationSpec.Name)
	}

	// Create a new opsRunner for this activity
	or := opsRunner{
		clusterName:        input.ClusterName,
		opsToRun:           []registry.OperationSpec{*origOpSpec},
		nodeCount:          input.NodeCount,
		seed:               input.Seed,
		logger:             l,
		datadogEventClient: datadogV1.NewEventsApi(datadog.NewAPIClient(datadog.NewConfiguration())),
		datadogTags:        input.DatadogTags,
	}
	or.status.running = make(map[string]struct{})
	or.status.lastRun = make(map[string]time.Time)

	if input.WorkloadClusterName != "" {
		or.workloadClusterName = input.WorkloadClusterName
		or.workloadNodes = input.WorkloadNodes
	}

	// Use the existing runOperation method to execute the operation
	return or.runOperation(ctx, origOpSpec, rand.New(rand.NewSource(input.Seed)), input.WorkerIdx)
}

// startOperationsWorkflow starts the Temporal workflow for running operations and keeps it running.
// This function doesn't execute any operations itself but sets up the workflow infrastructure.
func startOperationsWorkflow(clusterName string) error {
	l, err := logger.RootLogger("", logger.NoTee)
	if err != nil {
		return err
	}
	if err = roachprod.LoadClusters(); err != nil {
		return err
	}
	_, err = getCachedCluster(clusterName)
	if err != nil {
		return err
	}
	populateOpsSpec()
	// Create Temporal client
	c, err := client.Dial(client.Options{})
	if err != nil {
		return err
	}
	defer c.Close()

	// Start a worker to process activities
	w := worker.New(c, operationsTaskQueue+"-"+clusterName, worker.Options{})
	w.RegisterWorkflow(OperationsWorkflow)
	w.RegisterActivity(RunOperationActivity)

	// Start worker (non-blocking)
	err = w.Start()
	if err != nil {
		return err
	}
	defer w.Stop()

	// Execute workflow
	options := client.StartWorkflowOptions{
		ID:        operationsWorkflowID + "-" + clusterName,
		TaskQueue: operationsTaskQueue + "-" + clusterName,
	}

	we, err := c.ExecuteWorkflow(context.Background(), options, OperationsWorkflow, clusterName)
	if err != nil {
		return err
	}

	l.Printf("Workflow started with ID: %s and Run ID: %s", we.GetID(), we.GetRunID())
	l.Printf("Workflow is running. Press Ctrl+C to exit.")

	// Keep the process running until interrupted
	select {}
}

func filterOpsToRun(filter string) ([]SerializableOperationSpec, error) {
	regex, err := regexp.Compile(filter)
	if err != nil {
		return nil, err
	}
	var filteredOps []SerializableOperationSpec
	for _, opSpec := range originalOpSpecs {
		if regex.MatchString(opSpec.Name) {
			filteredOps = append(filteredOps, FromOperationSpec(opSpec))
		}
	}
	if len(filteredOps) == 0 {
		return nil, fmt.Errorf("no matching operations to run")
	}
	return filteredOps, nil
}
