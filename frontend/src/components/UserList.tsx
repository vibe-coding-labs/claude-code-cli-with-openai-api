import React, { useEffect, useMemo, useState } from 'react';
import {
  Button,
  Card,
  Form,
  Input,
  Modal,
  Select,
  Table,
  Tag,
  message,
} from 'antd';
import { ReloadOutlined, PlusOutlined } from '@ant-design/icons';
import { Link } from 'react-router-dom';
import { userAPI } from '../services/api';
import { User, UserCreateRequest, UserUpdateRequest } from '../types/user';
import './UserManagement.css';

const { Option } = Select;

const statusColor: Record<string, string> = {
  active: 'green',
  disabled: 'red',
};

const roleColor: Record<string, string> = {
  admin: 'blue',
  user: 'default',
};

const UserList: React.FC = () => {
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchText, setSearchText] = useState('');
  const [roleFilter, setRoleFilter] = useState<string>('');
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [createOpen, setCreateOpen] = useState(false);
  const [editUser, setEditUser] = useState<User | null>(null);
  const [resetUser, setResetUser] = useState<User | null>(null);
  const [creating, setCreating] = useState(false);
  const [updating, setUpdating] = useState(false);
  const [resetting, setResetting] = useState(false);
  const [createForm] = Form.useForm<UserCreateRequest>();
  const [editForm] = Form.useForm<UserUpdateRequest>();
  const [resetForm] = Form.useForm<{ password: string }>();

  const fetchUsers = async () => {
    setLoading(true);
    try {
      const data = await userAPI.listUsers();
      setUsers(data);
    } catch (error: any) {
      message.error(error.response?.data?.error || '获取用户列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchUsers();
  }, []);

  const filteredUsers = useMemo(() => {
    return users.filter((user) => {
      if (searchText && !user.username.toLowerCase().includes(searchText.toLowerCase())) {
        return false;
      }
      if (roleFilter && user.role !== roleFilter) {
        return false;
      }
      if (statusFilter && user.status !== statusFilter) {
        return false;
      }
      return true;
    });
  }, [users, searchText, roleFilter, statusFilter]);

  const handleCreate = async () => {
    try {
      const values = await createForm.validateFields();
      setCreating(true);
      await userAPI.createUser(values);
      message.success('用户创建成功');
      setCreateOpen(false);
      createForm.resetFields();
      fetchUsers();
    } catch (error: any) {
      if (error?.errorFields) return;
      message.error(error.response?.data?.error || '创建用户失败');
    } finally {
      setCreating(false);
    }
  };

  const handleEdit = async () => {
    if (!editUser) return;
    try {
      const values = await editForm.validateFields();
      setUpdating(true);
      await userAPI.updateUser(editUser.id, values);
      message.success('用户更新成功');
      setEditUser(null);
      editForm.resetFields();
      fetchUsers();
    } catch (error: any) {
      if (error?.errorFields) return;
      message.error(error.response?.data?.error || '更新用户失败');
    } finally {
      setUpdating(false);
    }
  };

  const handleResetPassword = async () => {
    if (!resetUser) return;
    try {
      const values = await resetForm.validateFields();
      setResetting(true);
      await userAPI.updateUserPassword(resetUser.id, { password: values.password });
      message.success('密码已重置');
      setResetUser(null);
      resetForm.resetFields();
    } catch (error: any) {
      if (error?.errorFields) return;
      message.error(error.response?.data?.error || '重置密码失败');
    } finally {
      setResetting(false);
    }
  };

  const handleToggleStatus = async (user: User) => {
    try {
      const newStatus = user.status === 'active' ? 'disabled' : 'active';
      await userAPI.updateUserStatus(user.id, { status: newStatus });
      message.success('状态已更新');
      fetchUsers();
    } catch (error: any) {
      message.error(error.response?.data?.error || '更新状态失败');
    }
  };

  const handleDelete = async (user: User) => {
    try {
      await userAPI.deleteUser(user.id);
      message.success('用户已删除');
      fetchUsers();
    } catch (error: any) {
      message.error(error.response?.data?.error || '删除用户失败');
    }
  };

  return (
    <Card
      title="用户管理"
      className="user-management"
      extra={
        <div className="user-management__toolbar">
          <Input.Search
            allowClear
            placeholder="搜索用户名"
            onSearch={setSearchText}
            onChange={(event) => setSearchText(event.target.value)}
            style={{ width: 220 }}
          />
          <Select
            allowClear
            placeholder="角色"
            value={roleFilter || undefined}
            onChange={(value) => setRoleFilter(value || '')}
            style={{ width: 140 }}
          >
            <Option value="admin">管理员</Option>
            <Option value="user">普通用户</Option>
          </Select>
          <Select
            allowClear
            placeholder="状态"
            value={statusFilter || undefined}
            onChange={(value) => setStatusFilter(value || '')}
            style={{ width: 140 }}
          >
            <Option value="active">启用</Option>
            <Option value="disabled">禁用</Option>
          </Select>
          <Button icon={<ReloadOutlined />} onClick={fetchUsers} loading={loading}>
            刷新
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
            新建用户
          </Button>
        </div>
      }
    >
      <Table
        rowKey="id"
        loading={loading}
        dataSource={filteredUsers}
        pagination={{ pageSize: 10 }}
        columns={[
          {
            title: '用户名',
            dataIndex: 'username',
          },
          {
            title: '角色',
            dataIndex: 'role',
            render: (value: User['role']) => <Tag color={roleColor[value]}>{value}</Tag>,
          },
          {
            title: '状态',
            dataIndex: 'status',
            render: (value: User['status']) => <Tag color={statusColor[value]}>{value}</Tag>,
          },
          {
            title: '创建时间',
            dataIndex: 'created_at',
          },
          {
            title: '操作',
            key: 'actions',
            render: (_, record: User) => (
              <div className="user-management__table-actions">
                <Button size="small" onClick={() => { setEditUser(record); editForm.setFieldsValue({ username: record.username, role: record.role, status: record.status }); }}>
                  编辑
                </Button>
                <Button size="small" onClick={() => setResetUser(record)}>
                  重置密码
                </Button>
                <Button size="small" onClick={() => handleToggleStatus(record)}>
                  {record.status === 'active' ? '禁用' : '启用'}
                </Button>
                <Button size="small" danger onClick={() => handleDelete(record)}>
                  删除
                </Button>
                <Button size="small" type="link">
                  <Link to={`/ui/users/${record.id}/usage`}>用量</Link>
                </Button>
              </div>
            ),
          },
        ]}
      />

      <Modal
        title="新建用户"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={handleCreate}
        confirmLoading={creating}
        destroyOnClose
      >
        <Form form={createForm} layout="vertical" initialValues={{ role: 'user', status: 'active' }}>
          <Form.Item label="用户名" name="username" rules={[{ required: true, min: 3 }]}>
            <Input placeholder="请输入用户名" />
          </Form.Item>
          <Form.Item label="密码" name="password" rules={[{ required: true, min: 6 }]}>
            <Input.Password placeholder="请输入密码" />
          </Form.Item>
          <Form.Item label="角色" name="role" rules={[{ required: true }]}>
            <Select>
              <Option value="admin">管理员</Option>
              <Option value="user">普通用户</Option>
            </Select>
          </Form.Item>
          <Form.Item label="状态" name="status" rules={[{ required: true }]}>
            <Select>
              <Option value="active">启用</Option>
              <Option value="disabled">禁用</Option>
            </Select>
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="编辑用户"
        open={!!editUser}
        onCancel={() => setEditUser(null)}
        onOk={handleEdit}
        confirmLoading={updating}
        destroyOnClose
      >
        <Form form={editForm} layout="vertical">
          <Form.Item label="用户名" name="username" rules={[{ required: true, min: 3 }]}>
            <Input placeholder="请输入用户名" />
          </Form.Item>
          <Form.Item label="角色" name="role" rules={[{ required: true }]}>
            <Select>
              <Option value="admin">管理员</Option>
              <Option value="user">普通用户</Option>
            </Select>
          </Form.Item>
          <Form.Item label="状态" name="status" rules={[{ required: true }]}>
            <Select>
              <Option value="active">启用</Option>
              <Option value="disabled">禁用</Option>
            </Select>
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="重置密码"
        open={!!resetUser}
        onCancel={() => setResetUser(null)}
        onOk={handleResetPassword}
        confirmLoading={resetting}
        destroyOnClose
      >
        <Form form={resetForm} layout="vertical">
          <Form.Item label="新密码" name="password" rules={[{ required: true, min: 6 }]}>
            <Input.Password placeholder="请输入新密码" />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
};

export default UserList;
