package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"

	"github.com/docker/distribution/registry/api/errcode"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	crierrors "k8s.io/cri-api/pkg/errors"

	"github.com/L-F-Z/TaskC/pkg/bundle"
	"github.com/L-F-Z/cri-t/internal/log"
	"github.com/L-F-Z/cri-t/server/metrics"
)

// PullImage pulls a image with authentication config.
func (s *Server) PullImage(ctx context.Context, req *types.PullImageRequest) (*types.PullImageResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	image := ""
	img := req.Image
	if img != nil {
		image = img.Image
	}
	log.Infof(ctx, "Pulling image: %s", image)

	pullArgs := pullArguments{image: image}

	sc := req.SandboxConfig
	if sc != nil {
		if sc.Linux != nil {
			pullArgs.sandboxCgroup = sc.Linux.CgroupParent
		}
		if sc.Metadata != nil {
			pullArgs.namespace = sc.Metadata.Namespace
		}
	}

	// We use the server's pullOperationsInProgress to record which images are
	// currently being pulled. This allows for avoiding pulling the same image
	// in parallel. Hence, if a given image is currently being pulled, we queue
	// into the pullOperation's waitgroup and wait for the pulling goroutine to
	// unblock us and re-use its results.
	pullOp, pullInProcess := func() (pullOp *pullOperation, inProgress bool) {
		s.pullOperationsLock.Lock()
		defer s.pullOperationsLock.Unlock()
		pullOp, inProgress = s.pullOperationsInProgress[pullArgs]
		if !inProgress {
			pullOp = &pullOperation{}
			s.pullOperationsInProgress[pullArgs] = pullOp
			pullOp.wg.Add(1)
		}
		return pullOp, inProgress
	}()

	if !pullInProcess {
		pullOp.err = errors.New("pullImage was aborted by a Go panic")
		defer func() {
			s.pullOperationsLock.Lock()
			delete(s.pullOperationsInProgress, pullArgs)
			pullOp.wg.Done()
			s.pullOperationsLock.Unlock()
		}()
		pullOp.bundleId, pullOp.err = s.pullImage(ctx, &pullArgs)
	} else {
		// Wait for the pull operation to finish.
		pullOp.wg.Wait()
	}

	if pullOp.err != nil {
		if errors.Is(pullOp.err, syscall.ECONNREFUSED) {
			return nil, fmt.Errorf("%w: %w", crierrors.ErrRegistryUnavailable, pullOp.err)
		}

		return nil, pullOp.err
	}

	log.Infof(ctx, "Pulled image: %v", pullOp.bundleId)
	return &types.PullImageResponse{
		ImageRef: string(pullOp.bundleId),
	}, nil
}

// pullImage performs the actual pull operation of PullImage. Used to separate
// the pull implementation from the pullCache logic in PullImage and improve
// readability and maintainability.
func (s *Server) pullImage(ctx context.Context, pullArgs *pullArguments) (bundle.BundleId, error) {
	var err error
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	name, err := bundle.ParseBundleName(pullArgs.image)
	if err != nil {
		return "", err
	}

	if deadline, ok := ctx.Deadline(); ok {
		log.Debugf(ctx, "Pull timeout is: %s", time.Until(deadline))
	}

	// TODO: Cancel the pull if no progress is made
	repoDigest, err := s.StorageService().PullImage(ctx, name)
	if err != nil {
		log.Debugf(ctx, "Error pulling image %s: %v", name, err)
		tryIncrementImagePullFailureMetric(err)
		return "", err
	}

	metrics.Instance().MetricImagePullsSuccessesInc()
	return repoDigest, nil
}

func tryIncrementImagePullFailureMetric(err error) {
	// We try to cover some basic use-cases
	const labelUnknown = "UNKNOWN"
	label := labelUnknown

	// Docker registry errors
	for _, desc := range errcode.GetErrorAllDescriptors() {
		if strings.Contains(err.Error(), desc.Message) {
			label = desc.Value
			break
		}
	}
	if label == labelUnknown {
		if strings.Contains(err.Error(), "connection refused") { //nolint:gocritic
			label = "CONNECTION_REFUSED"
		} else if strings.Contains(err.Error(), "connection timed out") {
			label = "CONNECTION_TIMEOUT"
		} else if strings.Contains(err.Error(), "404 (Not Found)") {
			label = "NOT_FOUND"
		}
	}

	// Update metric for failed image pulls
	metrics.Instance().MetricImagePullsFailuresInc(label)
}
