## Distributing Keen Code

1. How can we distribute Keen Code so that people can download and use it simply by typing `keen` on terminal? Share some ideas.
2. I want you to create a plan on how we can distribute this project through
    Github pipeline in homebrew, install script, and npm. For example, I want to
    enable users to be able to download keen-code like this:

    Install Claude Code:
        **MacOS/Linux (Recommended):**
        ```bash
        curl -fsSL https://claude.ai/install.sh | bash
        ```
        **Homebrew (MacOS/Linux):**
        ```bash
        brew install --cask claude-code
        ```
        **NPM (Deprecated):**
        ```bash
        npm install -g @anthropic-ai/claude-code 
3. Check the plan in @.ai-interactions/outputs/phase-4/output-5_distribution-plan.md. As a first step, we want to implement the support for npm-based installation. Let's implement this. First, explain how this will be implemented step by step.
4. Ok let's implement the changes. First, create a todo list for yourself and then implement one by one.
5. I have pushed the tag but pipeline failed due to test issues. Fixed them and pushed the latest commit to master. Now what?
6. I face this issue while trying to publish keen code on npm:
    ```
    npm ERR! code ENEEDAUTH
    npm ERR! need auth auth required for publishing
    npm ERR! A complete log of this run can be found in:
    npm ERR!     /Users/mochow/.npm/_logs/2026-03-22T12_25_51_461Z-debug-log.txt
    ```
    What should I do to fix this?

7. Ok now, let's create a new tag on Github, push it, and publish it as npm package.
8. When a tag is created and pushed, npm package should be published directly from Github Actions. How to achieve that?
9. We shouldn't actually use NPM_TOKEN for publishing npm package. We will use Trusted Publisher for it. Let's update the workflow to use Trusted Publisher.
10. The README.md doesn't appear on npm. How to fix that?
11. We want to support script-based install as outlined in @.ai-interactions/outputs/phase-4/output-2_distribution-plan.md. Let's implement this.
12. Currently, this project doesn't have any changelog. What's the best approach for creating and maintaining a changelog? We want this changelog to be updated after each release.
13. Ok let's go with manual CHANGELOG.md.