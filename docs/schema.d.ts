/* This file generates the `schema.json` file. */

export interface Schema {
  /** @default https://turborepo.org/schema.json */
  $schema?: string;

  /**
   * The base branch or your git repository. Git is used by turbo in its hashing algorithm
   * and --since CLI flag.
   *
   * @default origin/master
   */
  baseBranch?: string;

  /**
   * A list of globs and environment variables for implicit global hash dependencies.
   * Environment variables should be prefixed with $ (e.g. $GITHUB_TOKEN).
   *
   * Any other entry without this prefix, will be considered filesystem glob. The
   * contents of these files will be included in the global hashing algorithm and affect
   * the hashes of all tasks.
   *
   * This is useful for busting the cache based on .env files (not in Git), environment
   * variables, or any root level file that impacts package tasks (but are not represented
   * in the traditional dependency graph
   *
   * (e.g. a root tsconfig.json, jest.config.js, .eslintrc, etc.)).
   *
   * @default []
   */
  globalDependencies?: string[];

  /**
   * An object representing the task dependency graph of your project. turbo interprets
   * these conventions to properly schedule, execute, and cache the outputs of tasks in
   * your project.
   *
   * @default {}
   */
  pipeline: {
    /**
     * The name of a task that can be executed by turbo run. If turbo finds a workspace
     * package with a package.json scripts object with a matching key, it will apply the
     * pipeline task configuration to that NPM script during execution. This allows you to
     * use pipeline to set conventions across your entire Turborepo.
     */
    [script: string]: Pipeline;
  };
  /**
   * Configuration options that control how turbo interfaces with the remote Cache.
   * @default {}
   */
  remoteCache?: RemoteCache;
}

export interface Pipeline {
  /**
   * The list of tasks and environment variables that this task depends on.
   *
   * Prefixing an item in dependsOn with a ^ tells turbo that this pipeline task depends
   * on the package's topological dependencies completing the task with the ^ prefix first
   * (e.g. "a package's build tasks should only run once all of its dependencies and
   * devDependencies have completed their own build commands").
   *
   * Items in dependsOn without ^ prefix, express the relationships between tasks at the
   * package level (e.g. "a package's test and lint commands depend on build being
   * completed first").
   *
   * Prefixing an item in dependsOn with a $ tells turbo that this pipeline task depends
   * the value of that environment variable.
   *
   * @default []
   */
  dependsOn?: string[];

  /**
   * The set of glob patterns of a task's cacheable filesystem outputs.
   *
   * Note: turbo automatically logs stderr/stdout to .turbo/run-<task>.log. This file is
   * always treated as a cacheable artifact and never needs to be specified.
   *
   * Passing an empty array can be used to tell turbo that a task is a side-effect and
   * thus doesn't emit any filesystem artifacts (e.g. like a linter), but you still want
   * to cache its logs (and treat them like an artifact).
   *
   * @default ["dist/**", "build/**"]
   */
  outputs?: string[];

  /**
   * Whether or not to cache the task outputs. Setting cache to false is useful for daemon
   * or long-running "watch" or development mode tasks that you don't want to cache.
   *
   * @default true
   */
  cache?: boolean;
}

export interface RemoteCache {
  /**
   * The teamId used in requests to the Remote Cache.
   */
  teamId?: string;
  /**
   * Configuration options that control the integrity and authentication checks for
   * artifacts uploaded to and downloaded from the remote cache.
   *
   * @default {}
   */
  signature?: Signature;
}

export interface Signature {
  /**
   * Indicates if signature verification is enabled for requests to the remote cache. When
   * `enabled` is `true`, Turborepo will sign every uploaded artifact using the `key`.
   * Turborepo will reject any downloaded artifacts that have an invalid signature or are
   * missing a signature.
   *
   * @default false
   */
  enabled?: boolean;
  /**
   * The secret key to use for signing and verifying signatures on artifacts uploaded to
   * the remote cache.
   *
   * If both `key` and `keyEnv` are present, then `key` will be used.
   *
   * @default ""
   */
  key?: string;
  /**
   * The environment variable that contains the value of the secret key used for signing
   * and verifying signatures on artifacts uploaded to the remote cache.
   *
   * If both `key` and `keyEnv` are present, then `key` will be used.
   *
   * @default ""
   */
  keyEnv?: string;
}
