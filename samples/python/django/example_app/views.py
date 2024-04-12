from django.shortcuts import render

# a form view for our todos from our models
from django.views.generic.edit import FormView
from .models import Todo
from .forms import TodoForm

class TodoFormView(FormView):
    template_name = 'todo_form.html'
    form_class = TodoForm
    success_url = '/'

    def form_valid(self, form):
        form.save()
        return super().form_valid(form)
