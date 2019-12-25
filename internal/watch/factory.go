package watch

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/derailed/k9s/internal/client"
	"github.com/rs/zerolog/log"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	di "k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
)

// Factory - *factories(ns) -> *informers
const (
	defaultResync = 10 * time.Minute
	allNamespaces = ""
	clusterScope  = "-"
)

// Factory tracks various resource informers.
type Factory struct {
	factories  map[string]di.DynamicSharedInformerFactory
	client     client.Connection
	stopChan   chan struct{}
	activeNS   string
	forwarders Forwarders
}

// NewFactory returns a new informers factory.
func NewFactory(client client.Connection) *Factory {
	return &Factory{
		client:     client,
		stopChan:   make(chan struct{}),
		factories:  make(map[string]di.DynamicSharedInformerFactory),
		forwarders: NewForwarders(),
	}
}

func (f *Factory) Dump() {
	log.Debug().Msgf("----------- FACTORIES -------------")
	for ns := range f.factories {
		log.Debug().Msgf("  Factory for NS %q", ns)
	}
	log.Debug().Msgf("-----------------------------------")
}

func (f *Factory) Debug(gvr string) {
	log.Debug().Msgf("----------- DEBUG FACTORY (%s) -------------", gvr)
	inf := f.factories[allNamespaces].ForResource(toGVR(gvr))
	for i, k := range inf.Informer().GetStore().ListKeys() {
		log.Debug().Msgf("%d -- %s", i, k)
	}
}

func (f *Factory) Show(ns, gvr string) {
	log.Debug().Msgf("----------- SHOW FACTORIES %q -------------", ns)
	inf := f.ForResource(ns, gvr)
	for _, k := range inf.Informer().GetStore().ListKeys() {
		log.Debug().Msgf("  Key: %s", k)
	}
}

func (f *Factory) List(gvr, ns string, sel labels.Selector) ([]runtime.Object, error) {
	auth, err := f.Client().CanI(ns, gvr, []string{"list"})
	if err != nil {
		return nil, err
	}
	if !auth {
		return nil, fmt.Errorf("User has insufficient access to list %s", gvr)
	}

	inf := f.ForResource(ns, gvr)
	if inf == nil {
		return nil, fmt.Errorf("No resource for GVR %s", gvr)
	}
	if ns == clusterScope {
		return inf.Lister().List(sel)
	}

	return inf.Lister().ByNamespace(ns).List(sel)
}

func (f *Factory) Get(gvr, path string, sel labels.Selector) (runtime.Object, error) {
	ns, n := namespaced(path)
	auth, err := f.Client().CanI(ns, gvr, []string{"get"})
	if err != nil {
		return nil, err
	}
	if !auth {
		return nil, fmt.Errorf("User has insufficient access to get %s", gvr)
	}

	inf := f.ForResource(ns, gvr)
	if inf == nil {
		return nil, fmt.Errorf("No resource for GVR %s", gvr)
	}
	if ns == clusterScope {
		return inf.Lister().Get(n)
	}

	log.Debug().Msgf("GET %q--%q:%q", gvr, ns, path)
	return inf.Lister().ByNamespace(ns).Get(n)
}

func (f *Factory) WaitForCacheSync() {
	for _, fac := range f.factories {
		fac.WaitForCacheSync(f.stopChan)
	}
}

func (f *Factory) Init() {
	f.Start(f.stopChan)
}

func (f *Factory) Terminate() {
	if f.stopChan != nil {
		close(f.stopChan)
		f.stopChan = nil
	}
	for k := range f.factories {
		delete(f.factories, k)
	}
	f.forwarders.DeleteAll()
}

// DeleteForwarder deletes portforward for a given container.
func (f *Factory) DeleteForwarder(path string) {
	if fwd, ok := f.forwarders[path]; ok {
		fwd.Stop()
		delete(f.forwarders, path)
	}
}

// RegisterForwarder registers a new portforward for a given container.
func (f *Factory) RegisterForwarder(pf Forwarder) {
	f.forwarders[pf.Path()] = pf
}

// Forwards returns all portforwards.
func (f *Factory) Forwarders() Forwarders {
	return f.forwarders
}

// ForwarderFor returns a portforward for a given container or nil if none exists.
func (f *Factory) ForwarderFor(path string) (Forwarder, bool) {
	fwd, ok := f.forwarders[path]
	return fwd, ok
}

// Start initializes the informers until caller cancels the context.
func (f *Factory) Start(stopChan chan struct{}) {
	for ns, fac := range f.factories {
		log.Debug().Msgf("Starting factory in ns %q", ns)
		fac.Start(stopChan)
	}
}

// BOZO!! Check ns access for resource??
func (f *Factory) SetActive(ns string) {
	if !f.isClusterWide() {
		f.ensureFactory(ns)
	}
	f.activeNS = ns
}

func (f *Factory) isClusterWide() bool {
	_, ok := f.factories[allNamespaces]
	return ok
}

func (f *Factory) preload(ns string) {
	f.ForResource(ns, "v1/pods")
	f.ForResource(allNamespaces, "apiextensions.k8s.io/v1beta1/customresourcedefinitions")
	f.ForResource(clusterScope, "rbac.authorization.k8s.io/v1/clusterroles")
	f.ForResource(allNamespaces, "rbac.authorization.k8s.io/v1/roles")
}

func (f *Factory) FactoryFor(ns string) di.DynamicSharedInformerFactory {
	return f.factories[ns]
}

func (f *Factory) Preload(ns, gvr string) {
	_ = f.ForResource(ns, gvr)
}

func (f *Factory) ForResource(ns, gvr string) informers.GenericInformer {
	fact := f.ensureFactory(ns)
	inf := fact.ForResource(toGVR(gvr))
	fact.Start(f.stopChan)

	return inf
}

func (f *Factory) ensureFactory(ns string) di.DynamicSharedInformerFactory {
	if f.isClusterWide() {
		ns = allNamespaces
	}
	if fac, ok := f.factories[ns]; ok {
		return fac
	}

	f.factories[ns] = di.NewFilteredDynamicSharedInformerFactory(
		f.client.DynDialOrDie(),
		defaultResync,
		ns,
		nil,
	)
	f.preload(ns)

	return f.factories[ns]
}

func toGVR(gvr string) schema.GroupVersionResource {
	log.Debug().Msgf("GVR -- %q", gvr)
	tokens := strings.Split(gvr, "/")
	if len(tokens) < 3 {
		tokens = append([]string{""}, tokens...)
	}

	return schema.GroupVersionResource{
		Group:    tokens[0],
		Version:  tokens[1],
		Resource: tokens[2],
	}
}

// Client return the factory connection.
func (f *Factory) Client() client.Connection {
	return f.client
}

// ----------------------------------------------------------------------------
// Helpers...

func namespaced(n string) (string, string) {
	ns, po := path.Split(n)

	return strings.Trim(ns, "/"), po
}
