# Integrations

GrokifyQL is intended to be shared across Grokify and PlexusOne analytics
products while keeping product-specific execution outside the core module.

This repository is the Go engine. Integrations that require another runtime
should live outside the root Go module. Hibernate support should be implemented
in a Java-focused repository such as `github.com/grokify/grokifyql-java` or a
more specific `github.com/grokify/grokifyql-hibernate`.

## UIForge

UIForge uses GrokifyQL for Metabase-style Questions and ad-hoc analytics
queries.

The UIForge server:

- reads a neutral analytics catalog
- converts datasets and fields into a GrokifyQL schema
- asks SystemForge/SpiceDB for dataset and field authorization
- compiles that authorization into `grokifyql.Policy`
- checks policy before saving Questions and before executing ad-hoc queries

## OmniRoadmap

OmniRoadmap exposes product-roadmap analytics over a DoltDB-backed store. It can
act as a UIForge analytics provider or expose its own GrokifyQL-compatible API.

Recommended boundary:

- OmniRoadmap owns canonical roadmap data and tenant/product permissions.
- GrokifyQL owns AST parsing and policy checks.
- UIForge owns dashboard composition and saved Questions.

## Aha Studio

Aha Studio can expose Aha-imported data through a product-specific dialect name
such as `AQL` while reusing GrokifyQL internals.

## VisionStudio

VisionStudio can use the same pattern as OmniRoadmap when exposing DoltDB-backed
analytics to UIForge or other tools.

## Dialect Names

Product-specific dialect names such as `AQL` and `ORQL` can be useful in APIs
and UIs, but the shared AST and policy model should remain GrokifyQL-compatible.

## Java and Hibernate

Java/Hibernate should be treated as an external integration target rather than
part of the Go engine.

Recommended boundary:

- The Go engine defines the query language, AST semantics, policy model, and
  security invariants.
- A Java module can implement Java-native AST bindings, Criteria/Hibernate
  compilation, and JVM package conventions.
- Cross-runtime compatibility should use documented AST/policy semantics and
  conformance tests instead of importing JVM dependencies into the Go module.
