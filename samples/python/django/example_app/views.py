from django.shortcuts import get_object_or_404, redirect, render

# a form view for our todos from our models
from django.views import View
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
    
    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['todos'] = Todo.objects.all()
        return context
    
# Toggle todo completed FormView
class ToggleTodoView(View):
    success_url = '/'
    
    def post(self, request, *args, **kwargs):
        todo = get_object_or_404(Todo, pk=self.kwargs['pk'])
        todo.completed = not todo.completed
        todo.save()
        return redirect(self.success_url)
    
# Toggle todo completed FormView
class DeleteTodoView(View):
    success_url = '/'
    
    def post(self, request, *args, **kwargs):
        todo = get_object_or_404(Todo, pk=self.kwargs['pk'])
        todo.delete()
        return redirect(self.success_url)
