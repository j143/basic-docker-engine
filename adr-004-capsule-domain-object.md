# ADR-004: Promote Capsule to the central domain object

## Status

Accepted

## Context

The original `ResourceCapsule` stored only a name, version, and path. It could not own resources, track members, participate in lifecycle transitions, or prevent deletion while resources were still in use.

At the same time, `CreateNetwork` registered a Capsule, but `DeleteNetwork` did not remove or reconcile that Capsule. This created two parallel identities for the same conceptual resource.

## Decision

A `Capsule` is now the central ownership and lifecycle boundary for engine resources.

A Capsule:

- owns one or more resources, such as a network;
- tracks active members, such as attached containers;
- has an explicit lifecycle status;
- is persisted durably;
- cannot be deleted while it has active members.

Networks are owned by Capsules. Attaching a container to a network also attaches that container to the network's Capsule. Deleting a network deletes its Capsule only after every container has been detached.

## Consequences

- Network deletion is now reference-safe.
- Capsule membership provides a foundation for future CPU, memory, PID, storage, and network-policy limits.
- The legacy `ResourceCapsule` remains temporarily for compatibility with existing Kubernetes-related commands.
- Real networking, cgroup enforcement, and container lifecycle integration remain separate follow-up work.

## Non-goals

This ADR does not claim that Capsules currently enforce resource limits or provide kernel-level isolation. They establish ownership, membership, persistence, and deletion semantics first.
