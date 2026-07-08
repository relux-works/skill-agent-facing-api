---
title: Agent-Facing API
summary: A design pattern for CLI query layers that are cheap for agents to read.
category: Agent infrastructure
featured: true
---

## What it is

A design pattern and reference implementation for building agent-optimized read layers
on top of an existing CLI. Instead of parsing verbose, human-formatted output, an agent
gets a token-efficient interface: a mini-query DSL for structured reads, a mutation
grammar with safety flags, and scoped grep for full-text search.

## Why it matters

CLI output built for humans costs agents 1.5–5x more tokens than they need. MCP servers
fix the format but add roughly 2,200 tokens of session overhead per connection and do
not batch — the break-even is hundreds of queries per session, which real work rarely
reaches. The two-layer read approach delivers the same clean JSON as MCP with none of
that overhead, and it composes: many queries in one call. This is the pattern that powers
our own agent gateway at api.relux.works.

## Who it is for

Anyone building tools that both humans and agents use, who wants agents to read them
efficiently without standing up separate infrastructure.
