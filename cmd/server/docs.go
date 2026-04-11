// Package main Chat Service API
//
// Real-time chat service for Modami. Provides conversation and message
// management with Centrifugo-backed live updates, Kafka event publishing,
// and ScyllaDB persistence.
//
// Authentication:
//   All /api/v1/* endpoints require a Bearer JWT in the Authorization header.
//
//	@title          Modami Chat Service
//	@version        1.0.0
//	@description    Real-time chat API with conversations, messages, reactions, and presence.
//
//	@contact.name   Modami Engineering
//	@contact.email  dev@modami.com
//
//	@license.name   Private
//
//	@host       localhost:8080
//	@BasePath   /api/v1
//
//	@securityDefinitions.apikey BearerAuth
//	@in                         header
//	@name                       Authorization
//	@description                JWT Bearer token. Format: "Bearer <token>"
package main
