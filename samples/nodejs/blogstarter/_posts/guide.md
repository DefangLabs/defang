---
title: "How to leverage this template"
excerpt: "Built on Next.js and Markdown, Defang offers a template that you can quickly adapt. By replacing excerpts, like this paragraph, with your own message to your audience, and swapping out images for those you wish to share with your fans, you can easily prepare your blogs locally. Wait, hang on for a second—do you mean LOCALLY? Yes, your blog won't be visible to others on their end. But don't worry; Defang has a solution. Click the title to learn how to deploy your blogs globally in just 10 minutes."
coverImage: "/assets/blog/guide/cover.jpg"
date: "2024-04-03T06:35:07.322Z"
author:
  name: Hongchen Yu
  picture: "/assets/blog/authors/hongchen.png"
ogImage:
  url: "/assets/blog/guide/cover.jpg"
---
We created this template to help you quickly launch your blogs. This template is built using Markdown and Next.js. We convert Markdown files to HTML string by using `remark` and`remark-html`. You can easily add new blogs or modify existing content (like this paragraph) by simply adding or changing the markdown files. We helped you to build all necessary files for deploying it through Defang. By simply downloading *[Defang](https://github.com/defang-io/defang)* and running `defang login` and `defang compose up` in the command line interface on your local machine, your blog will be available to everyone in the world.

Here is a detailed guide:

### How to edit the blog
1. Copy the code from our *[Github Repository](https://github.com/defang-io/defang/tree/main/samples/nodejs)* to your local machine
2. In "_posts", there are three existing Markdown files, representing what is currently shown to you in the template. You could replace the content within.
3. If you want to replace the cover image, you have to firstly add your image to the code. Then, find the "coverImage" tag and replace the directory (for example, "/assets/blog/exploration/cover.jpg") to the directory of your image.
4. You may find other tags within the Markdown file:
- Exerpt
The exerpt tag refers to the summary you would see at the main page. 
- Date
The date tag refers to the time the current blog is created. The earliest blog will be placed at the top.
- Author
The author tag refers to the name and the icon you would see beneath the blog title. You could replace them to your designated name and profile icon. 

### How to quickly deploy it with Defang
1. Download *[Defang](https://github.com/defang-io/defang)*
2. Type `defang login` in your command line interface
3. Type `defang compose up` and you are good to go!

### Limitations
1. We convert Markdown to HTML before rendering it to your screen. HTML tags within the Markdown file (for example, `<a href></a>`) will not work. 


