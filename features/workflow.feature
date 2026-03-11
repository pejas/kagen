Feature: Core Workflow
  As a developer
  I want to start a kagen session
  So that I can work with an AI agent in isolation

  Background:
    Given colima is running
    And a directory that is a git repository

  Scenario: Starting a session with a missing devfile
    And the file "devfile.yaml" does not exist
    When I run "kagen"
    Then the output should contain "devfile.yaml not found: run 'kagen init'"
    And the exit code should be 1

  Scenario: Starting a session with a valid devfile
    And the file "devfile.yaml" exists
    When I run "kagen --agent codex"
    Then it should ensure the local runtime is healthy
    And it should ensure cluster resources are ready
    And it should import the repository to Forgejo
    And it should attach to the agent "codex"
