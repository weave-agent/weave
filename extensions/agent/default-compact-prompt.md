Summarize the following conversation into a structured compaction summary. The summary must preserve all information needed for the agent to continue working seamlessly.

Format your summary as follows:

## Goal
What the user is trying to accomplish (1-3 sentences).

## Progress
What has been done so far, including:
- Key decisions made and their rationale
- Files that were created, modified, or deleted (with brief descriptions of changes)
- Current state of any in-progress work
- Any unresolved issues, errors, or blockers

## Active Constraints
Preserve ALL user-stated constraints verbatim in this section. Do not paraphrase or summarize constraints.
- List each constraint exactly as the user stated it
- Include any architectural patterns, style guides, or requirements the user specified
- Include performance targets, compatibility requirements, or external dependencies
- If the user rejected an approach or expressed a preference, record it here exactly

## Current Plan
Include the current plan state. If the user stated a multi-step goal, list completed and remaining steps as "step X of Y".
- State the overall plan in 1 sentence
- List completed steps with brief outcomes
- List remaining steps in order
- Note any steps that were modified, skipped, or added during the conversation

## Key Context
Important details the agent needs to remember:
- Architecture or design patterns being followed
- Constraints or requirements discussed
- Names of key types, functions, or variables
- Test requirements or coverage notes

## Recent Tool Activity
List the most recent tool calls and their outcomes (last 5-10 operations).

Rules:
- Be specific: include file paths, function names, error messages
- Be concise: omit pleasantries and restated instructions
- Preserve any code snippets, commands, or configuration values that were discussed
- If the user expressed preferences or corrections, include those verbatim
- Do not invent information not present in the conversation
