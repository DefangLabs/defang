import { ActionFunctionArgs, json, redirect } from "@remix-run/node";
import { Form, Outlet } from "@remix-run/react";
import { prisma } from "~/client";

export async function action({ request }: ActionFunctionArgs) {
  const formData = await request.formData();
  const newNote = await prisma.note.create({
    data: {
      title: formData.get("title")?.toString() || "",
      body: formData.get("body")?.toString() || "",
    },
  });

  return redirect(`/notes/${newNote.id}`);
}

export default function Notes() {
  return (
    <div style={{ display: "flex", flexDirection: "row", gap: "20px" }}>
      <div style={{ width: "33%" }}>
        <h1>Notes</h1>
        <Form method="post">
          <div
            style={{ display: "flex", flexDirection: "column", gap: "10px" }}
          >
            <input type="text" name="title" placeholder="Title" />
            <textarea name="body" placeholder="Body"></textarea>
            <button type="submit">Create</button>
          </div>
        </Form>
      </div>
      <div>
        <Outlet />
      </div>
    </div>
  );
}
