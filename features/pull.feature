Feature: Synchronizing Changes
  As a developer
  I want to pull reviewed changes from the cluster
  So that my local repository is updated with agent work

  Background:
    Given colima is running
    And a directory that is a git repository

  Scenario: Pulling changes with unpushed local work
    When I run "kagen start codex"
    Then it should ensure the local runtime is healthy
    And it should import the repository to Forgejo
    And it should attach to the agent "codex"

    When there are uncommitted local changes
    And I run "kagen pull"
    Then it should create a WIP commit
    And it should fetch changes from Forgejo
    And the output should contain "Successfully fast-forwarded reviewed changes"
    And the exit code should be 0
