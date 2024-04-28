from django.contrib import admin
from django.contrib import admin
from .models import Todo

@admin.register(Todo)
class ToDoAdmin(admin.ModelAdmin):
    pass