# Threat model

Treestamp is a local, read-only library. It is not a sandbox, a secret
redactor, or an atomic repository snapshot.

This text follows the pinned weavatrix-scan threat model. A short comment
elsewhere must not be upgraded into a stronger guarantee.

## Ordinary changing worktree

Discovery, open, and post-read checks are separate operations. A file may
grow, shrink, or be replaced between them. Version evidence and hashes detect
many races after bytes were already read. They do not prove that bytes from
outside the intended path were never opened.

A sequential walk is not an atomic snapshot. Per-file version evidence binds
one opened file, not one repository-wide moment.

## Adversarial concurrent mutation

A check/open window exists. Symlink and canonical-path checks are not atomic
with the later open. This document does not claim a proven exploit. Callers
that index untrusted trees while another party can mutate parents, roots, or
directory entries should treat the default backend as a detector, not a
preventer.

An `os.Root` hardened backend, if added, is a different profile. It can
narrow some traversal races. It does not make the library a sandbox and does
not remove every filesystem caveat. Its overhead is measured as B13, not
hidden in the default headline.

## Portable reports

A portable encoding may drop host paths, identities, timestamps, and free-form
diagnostics. Relative names remain. That is a trust-boundary encoding, not
secret redaction.

## Cooperative cancellation

Cancellation is checked between read chunks. A read already blocked in the
kernel is not interrupted.

## Walker (P1)

The serial walker skips descendants when it marks path escape, filesystem
boundary, symlink loop, or max depth. Those marks are best-effort on a live
tree. They are not a proven exclusive lock on the namespace.
