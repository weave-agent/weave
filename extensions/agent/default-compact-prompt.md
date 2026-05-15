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
