
## Setup
This sample requires an API key to access the OpenAI API. The name of the config value is referenced in the docker-compose.yml file. To provide a value for it, you can use the Defang CLI like this:

```
defang config set --name OPENAI_KEY
```

and then enter the value when prompted.


## Testing
```
echo "Hello" | curl -H "Content-Type: application/text" -d @- https://xxxxxxxx/prompt
```
or
```
cat prompt.txt | curl -H "Content-Type: application/text" -d @- https://xxxxxxxx/prompt
```