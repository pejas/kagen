Feature: Core Workflow
  As a developer
  I want to start a kagen session
  So that I can work with an AI agent in isolation

  Background:
    Given colima is running
    And a directory that is a git repository

  Scenario: Starting a session with the generated runtime
    When I run "kagen start codex"
    Then it should ensure the local runtime is healthy
    And it should ensure cluster resources are ready
    And it should import the repository to Forgejo
    And it should attach to the agent "codex"
    And the exit code should be 0
