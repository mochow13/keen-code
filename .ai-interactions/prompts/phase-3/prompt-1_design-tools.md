## Keen Code

This is Keen Code, a CLI coding agent that helps users write code. So far, the project has completed phase-2 of development. We now have a REPL, LLM integration, markdown rendering and some fundamental UI features.

## Design Tools

1. In phase-3 of the development, we want to enhance the LLMs with tools. Right now we are using `genkit` for LLM integration. As a first step, let's explore what support `genkit` has for tools and how we can leverage its capabilities out of the box.
2. In the first step, we want to integrate tool calling with the LLM with a dummy tool. Through this process, we want to 
  - create a common tool interface outlining critical methods
  - make the dummy tool implement the interface
  - integrate the tool with the LLM
  - stream both the messages and tool calling in real-time to the user
To achieve this, let's first draft a architecture document and persist it in @.ai-interactions/outputs/phase-3/output-1_design-tools.md.
3. Does Genkit not have a tool interface?
4. Let's say in future we will have a different LLM framework like langchain-go. Which approach would make sense the most if we want to use multiple frameworks?
5. We want to start with the most basic tool needed: `readFile`. It should be able to read a file and return its content. Then the agent will pass this content to LLM.
6. How does it work in `GenerateStream`? Does the stream stop/pause when LLM requests a tool call?


## Review

1. This is Keen Code, a CLI coding agent that helps users write code. So far, the project has completed phase-2 of development. We now have a REPL, LLM integration, markdown rendering and some fundamental UI features. As a next step, we want to enhance the LLMs with tools. I want you to explore the current state of the codebase. Then review the design document @.ai-interactions/outputs/phase-3/output-1_design-tools.md and provide feedback. In this design doc, we have focused on building the foundation for tools using a dummy tool and Genkit support. So keep that in mind while reviewing the design document.
2. Hi Kimi, there is a review on the design in @.ai-interactions/outputs/phase-3/output-2_tool-design-review.md for the design in @.ai-interactions/outputs/ph
ase-3/output-1_design-tools.md. Check the review and tell me what reviews make sense to incorporate.