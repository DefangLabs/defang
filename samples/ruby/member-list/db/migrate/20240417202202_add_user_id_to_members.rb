class AddUserIdToMembers < ActiveRecord::Migration[7.1]
  def change
    add_column :members, :user_id, :integer
    add_index :members, :user_id
  end
end