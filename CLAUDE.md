# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A coding agent framework written in Go — event-driven, extension-based, with dynamic compilation of selected extensions at runtime. Currently in design phase.

## Core Principle

Standard library as much as possible. Every replaceable component is an extension (runner, provider, tools, store, hooks). Extensions are independent Go modules that self-register via `init()`.

The architecture follows a launcher pattern: resolve config → pick extensions → build a custom binary (cached per hash) → exec it. Extensions import `sdk/`, which defines the interfaces and the `Wire()` composition root. A channel-based event bus connects everything.

## Design Reference

`docs/design.md` is **strong inspiration, not direct instruction**. It captures the architectural intent and data flow, but implementation details will evolve. Treat it as a north star, not a spec to copy verbatim.
