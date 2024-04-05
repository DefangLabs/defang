---
title: "How to leverage this template"
excerpt: "Based on Next.js and Markdown, Defang provides you with a template that you could quickly get your hands on. By replacing the excerpts, like this paragraph, to something you want to say to your audience, and, of course, the images to what you want to show to your fans, you could quickly get your blogs ready locally. Wait, hang on for a second, do you mean LOCALLY? Yes, other people cannot see your blog on their end. Don't worry. Defang helps you out. Click the title to know how to deploy your blogs globally in just 10 minutes."
coverImage: "/assets/blog/guide/cover.jpg"
date: "2024-04-03T06:35:07.322Z"
author:
  name: Hongchen Yu
  picture: "/assets/blog/authors/hongchen.png"
ogImage:
  url: "/assets/blog/guide/cover.jpg"
---
We create this template to help you quickly launch your blog. This template is built using Markdown and Next.js. We convert Markdown files to HTML string by using `remark` and`remark-html`. You could easily add new blogs or modify existing content (like this paragraph) by simply adding or changing the markdown files. We helped you to build all necessary files for deploying it through Defang. By simply downloading [Defang](https://github.com/defang-io/defang) and running `defang login` and `defang compose up` in the command line interface on your local machine, your blog will be available to everyone in the world.

Here is a detailed guide:

### How to edit the blog
1. Copy the code from our [Github Repository](https://github.com/defang-io/defang/tree/main/samples/nodejs) to your local machine
2. In "_posts", there are three existing Markdown files, representing what is currently shown to you in the template. You could replace the content within.
3. If you want to replace the cover image, you have to firstly add your image to the code. Then, find the "coverImage" tag and replace the directory (for example, "/assets/blog/dynamic-routing/cover.jpg") to the directory of your image.
4. You may find other tags within the Markdown file:
- Exerpt
The exerpt tag refers to the summary you would see at the main page. 
- Date
The date tag refers to the time the current blog is created. The earliest blog will be placed at the top.
- Author
The author tag refers to the name and the icon you would see beneath the blog title. You could replace them to your designated name and profile icon. 

### How to quickly deploy it with Defang
1. Download [Defang](https://github.com/defang-io/defang)
2. Type `defang login` in your command line interface
3. Type `defang compose up` and you are good to go!

### Limitations
1. If the images you add to the code is a local copy, you will not be able to adjust its size due to Next.js limitations.
2. We convert Markdown to HTML before rendering it to your screen. HTML tags within the Markdown file (for example, <a href></a>) will not work. 


