package controller

const (
	EgressSidecarNamePrefix  = "egress-sidecar"
	IngressSidecarNamePrefix = "ingress-sidecar"
	AnnotationIdentity       = "aegis/identity"
	AnnotationIngress        = "aegis/identity.allowed"
	AnnotationIngressClaim   = "aegis/identity.claim" // check on claim
	AnnotationEgress         = "aegis/egress"
)
