# ImgProxy

ImgProxy is a fast and secure standalone server for resizing and converting remote images. It's can be deployed using their official Docker image, as documented [here](https://docs.imgproxy.net/installation#docker).

## Prerequisites

Install the Defang CLI by following the instructions in the [Defang CLI documentation](https://docs.defang.io/docs/getting-started).

## Build and run the application

If you have environment variables configured for your [own cloud account](https://docs.defang.io/docs/concepts/defang-byoc), this will deploy the application to your cloud account, otherwise it will deploy to the Defang cloud.

```sh
defang compose up
```
