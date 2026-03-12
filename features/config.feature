Feature: Optional repository configuration
  As a developer
  I want to write optional repository defaults with kagen
  So that I can keep repo-specific agent settings without bootstrapping the runtime

  Scenario: Write optional project configuration
    Given a directory that is a git repository
    When I run "kagen config write"
    Then the file ".kagen.yaml" should exist
    And the output should contain "Wrote ... .kagen.yaml"
