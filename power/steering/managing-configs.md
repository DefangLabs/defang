# Managing Configs

Configs are key-value pairs that can be used to store sensitive information such as API keys, database credentials, or any other configuration data that should not be hardcoded into your application.

## Viewing Configs

To view the current configs for your project, use the following MCP tool:

```bash
list_configs
```

This will display a list of all the configs currently set for your project. This list does not represent configs you are still required to set; it only shows what has already been set.

## Setting Configs

To set a config, use the following MCP tool:

```bash
set_config
```

**IMPORTANT**:
When using the `set_config` tool, ensure that only one of the following options is provided: either the `value` parameter or the `random` flag.
Providing both will result in an error.
Example:
`bash
        {
        "name": "POSTGRES_PASSWORD",
        "value": "helloworld123",
        "random": true,
        "working_directory": "."
        }
        `

Available parameters:

- `name` (required): The key for the config you want to set.
- `value` (optional): The value for the config. Do not provide this parameter if you are using the `random` parameter.
  Example:
  ```bash
  {
  "name": "POSTGRES_PASSWORD",
  "value": "helloworld123",
  "working_directory": "."
  }
  ```
- `random` (optional): If this flag is provided, a random value will be generated for the config. Do not provide the `value` parameter if you are using this parameter.
  Example:
  `bash
   {
   "name": "POSTGRES_PASSWORD",
   "random": true,
   "working_directory": "."
   }
   `

## Deleting Configs

To delete a config, use the following MCP tool:

```bash
remove_config
```

This will remove the specified config from your project.

## Best Practices

- Avoid hardcoding sensitive information in your codebase. Use configs instead.
- Regularly rotate sensitive configs such as API keys and passwords.
- Use descriptive keys for your configs to make them easily identifiable.
