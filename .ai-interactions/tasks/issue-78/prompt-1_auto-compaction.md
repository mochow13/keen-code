## Auto Compaction

1. We want to implement automatic compaction. What are the possible approaches right now?

2. Pre-turn compaction is not the bigger problem right now. Since we show a suggestion to users, users can already choose to compact before sending the next message. Our problem is when agent turn is running and context window gradually shrinks. For example, let's say we are running a Ralph loop. Agent is looping over the set of tasks. Obviously, context will grow here within the loop. How can we make sure whenever context reaches to a certain threshold, automatically compaction is done, and also we let users know in the UI that compaction is ongoing ("Compacting..."). So within agent turn, we need to be able to trigger the compaction. How do you solve it?

3. I think we should simply use our existing compaction. It's simpler, we don't need to handle multi-provider nuances, and generalises among providers.

4. Here's how auto compaction will work:
   - auto trigger from LLM loop on 85% threshold
   - also compact if we face too big context error
   - retain last user message since it's the latest task
   - compact everything else, summarise user and agent messages, tool results, just like how we do today
   - we can use the same prompt mostly but tweak it to accommodate auto-compacting nuances
   - if needed, I am open to update existing compactionPrompt in `internal/llm/systemprompt.go` to make the process more robust and useful
   - of course, we will show "Compacting..." just like how we do now when compaction runs automatically. It will override existing loading spinner and text

5. The compaction format: `[compacted full history] + [last user message] = new after compaction user message`.

6. For too-big-context recovery, compact and retry the failed request once.

7. Automatic compaction should support both the interactive REPL and `keen run` / headless mode.

8. Show `Context compacted automatically.` for a few seconds, like `Copied to clipboard`.

9. Make the threshold 90%, measured against the effective input budget because safety margins are already excluded.

10. In the compacted message, mention that context has been compacted and annotate that the preserved message is the last user message.

11. Do not allow cancelling when auto compaction is running, because it is risky.

12. Automatic compaction output should not be shown. The agent should compact and continue its loop without breaking the loop.

13. A fixed timeout can be tricky: a model can be slow but still compacting and users can be patient. Allow compaction cancellation instead: `Esc` cancels compaction. Since state is not mutated with compacted input until it finishes, cancellation does not lose input.
