package federation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

type HubServerConfig struct {
	Region            string
	MaxLeaves         int
	ListenAddr        string
	HeartbeatTimeout  time.Duration
	GroupEngine       *GroupEngine
	ConfigDistributor *ConfigDistributor
	Logger            zerolog.Logger
}

type HubServer struct {
	cfg          HubServerConfig
	mu           sync.RWMutex
	leaves       map[string]*LeafState
	configPushCh map[string]chan *FedConfigUpdate
}

func NewHubServer(cfg HubServerConfig) *HubServer {
	return &HubServer{
		cfg:          cfg,
		leaves:       make(map[string]*LeafState),
		configPushCh: make(map[string]chan *FedConfigUpdate),
	}
}

func (s *HubServer) Register(ctx context.Context, req *FedAgentRegistration) (*FedRegisterResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.leaves) >= s.cfg.MaxLeaves {
		return &FedRegisterResponse{
			Accepted:        false,
			RejectionReason: fmt.Sprintf("hub capacity full (%d/%d)", len(s.leaves), s.cfg.MaxLeaves),
		}, nil
	}

	leaf := &LeafState{
		AgentID:    req.AgentId,
		Hostname:   req.Hostname,
		IP:         req.Ip,
		Version:    req.Version,
		Labels:     req.Labels,
		AutoLabels: req.AutoLabels,
		LastSeen:   time.Now(),
		Status:     LeafStatusOnline,
	}

	groups := s.cfg.GroupEngine.Evaluate(leaf)
	leaf.Groups = groups
	s.leaves[req.AgentId] = leaf
	s.cfg.GroupEngine.UpdateLeaf(leaf)

	configVersion, _ := s.cfg.ConfigDistributor.GetConfigVersion(req.AgentId, s.cfg.Region, groups)
	leaf.ConfigVersion = configVersion
	s.configPushCh[req.AgentId] = make(chan *FedConfigUpdate, 10)

	s.cfg.Logger.Info().Str("agent_id", req.AgentId).Strs("groups", groups).Msg("Leaf registered")

	return &FedRegisterResponse{
		Accepted:       true,
		AssignedRegion: s.cfg.Region,
		AssignedGroups: groups,
		ConfigVersion:  configVersion,
	}, nil
}

func (s *HubServer) Heartbeat(ctx context.Context, req *FedHeartbeatRequest) (*FedHeartbeatResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	leaf, ok := s.leaves[req.AgentId]
	if !ok {
		return &FedHeartbeatResponse{Ok: false}, nil
	}

	leaf.LastSeen = time.Now()
	leaf.Status = LeafStatusOnline

	if len(req.Labels) > 0 {
		leaf.Labels = req.Labels
		s.cfg.GroupEngine.UpdateLeaf(leaf)
	}

	configVersion, _ := s.cfg.ConfigDistributor.GetConfigVersion(req.AgentId, s.cfg.Region, leaf.Groups)
	updateAvailable := configVersion != leaf.ConfigVersion

	return &FedHeartbeatResponse{
		Ok:                    true,
		ConfigVersion:         configVersion,
		ConfigUpdateAvailable: updateAvailable,
	}, nil
}

func (s *HubServer) GetLeaves() map[string]*LeafState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]*LeafState, len(s.leaves))
	for k, v := range s.leaves {
		result[k] = v
	}
	return result
}
