# Slackbot

This is a simple slackbot that takes a request and posts the message from the body to a slack channel.

## Prerequisites

Install the Defang CLI by following the instructions in the [Defang CLI documentation](https://docs.defang.io/docs/getting-started).

### Slack API Token

You'll need to head to https://api.slack.com/apps to create a Slack App.

Make sure to:
 * Give it the bot `chat:write` scope
 * Install the app to your workspace
 * Copy the Bot User OAuth Access Token
 * Invite your bot to the channel you want it to post to using the `@botname` command in the channel (this will allow you to invite it)

## Configure

Before deploying the Slackbot, you need to set up some config values. These config values are environment variables that the Slackbot needs to function correctly. The values are:

- `SLACK_TOKEN`: This is the token you copied previously for the Slack API.
- `SLACK_CHANNEL_ID`: This is the ID of the Slack channel where the bot will post messages.

You can set these config parameters using the `defang config set` command. Here's how:

```sh
defang config set --name SLACK_TOKEN --value your_slack_token
defang config set --name SLACK_CHANNEL_ID --value your_slack_channel_id
```

## Deploy

If you have environment variables configured for your [own cloud account](https://docs.defang.io/docs/concepts/defang-byoc), this will deploy the application to your cloud account, otherwise it will deploy to the Defang playground.

```sh
defang compose up
```

## Usage

Once the Slackbot is deployed, you can send a POST request to the `/` endpoint with a JSON body containing the message you want to post to the Slack channel. Here's an example:

```sh
curl 'https://raphaeltm-bot--8080.prod1.defang.dev/' \
  -H 'content-type: application/json' \
  --data-raw $'{"message":"This is your bot speaking. We\'ll be landing in 10 minutes. Please fasten your seatbelts."}'
```